// Package speed measures why home Wi-Fi feels slow. Everything is read-only:
// it reads what the OS already knows, pings your own gateway, times DNS
// lookups, and (only with --load) downloads test data to check bufferbloat.
package speed

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kavinbm16/fixmyrouter/internal/checks"
)

// Report holds the raw measurements. Advice lives in Findings.
type Report struct {
	GatewayIP   string       `json:"gateway_ip"`
	WiFi        *WiFi        `json:"wifi,omitempty"`
	WiFiNote    string       `json:"wifi_note,omitempty"`
	Congestion  *Congestion  `json:"congestion,omitempty"`
	Gateway     *PingStats   `json:"gateway_latency,omitempty"`
	DNS         []DNSResult  `json:"dns,omitempty"`
	Bufferbloat *Bufferbloat `json:"bufferbloat,omitempty"`
	BloatNote   string       `json:"bufferbloat_note,omitempty"`
}

// Options control which measurements run.
type Options struct {
	Internet  bool // DNS comparison against public resolvers
	Load      bool // bufferbloat test (downloads up to 200 MB for ~10 s)
	DNSRounds int
}

// Run measures Wi-Fi and gateway latency, plus DNS and bufferbloat if asked.
// The slow OS Wi-Fi query runs alongside the network measurements.
func Run(ctx context.Context, gatewayIP string, opt Options) *Report {
	r := &Report{GatewayIP: gatewayIP}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if w, ok := DetectWiFi(); ok {
			r.WiFi = &w
			if c, ok := Analyze(w); ok {
				r.Congestion = &c
			}
		} else {
			r.WiFiNote = "Not on Wi-Fi, or the OS did not report Wi-Fi details."
		}
	}()
	if p, _ := Ping(ctx, gatewayIP, 20); p.Received > 0 {
		r.Gateway = &p
	}
	if opt.Internet {
		if opt.DNSRounds <= 0 {
			opt.DNSRounds = 3
		}
		if sys := SystemResolvers(); len(sys) > 0 {
			d := MeasureDNS(ctx, "Your current DNS", sys[0], opt.DNSRounds)
			d.System = true
			r.DNS = append(r.DNS, d)
		}
		for _, p := range PublicResolvers {
			r.DNS = append(r.DNS, MeasureDNS(ctx, p.Name, p.IP, opt.DNSRounds))
		}
	}
	if opt.Load {
		b, err := MeasureBufferbloat(ctx, "1.1.1.1", 10*time.Second)
		if err != nil {
			r.BloatNote = "Bufferbloat test failed: " + err.Error()
		} else {
			r.Bufferbloat = &b
		}
	} else if opt.Internet {
		r.BloatNote = "Skipped. Add --load to test lag under load (downloads up to 200 MB over ~10 s)."
	}
	wg.Wait()
	return r
}

// SignalQuality turns RSSI into words.
func SignalQuality(dbm int) string {
	switch {
	case dbm == 0:
		return "unknown"
	case dbm >= -60:
		return "excellent"
	case dbm >= -67:
		return "good"
	case dbm >= -75:
		return "fair"
	}
	return "weak"
}

func warn(id, sev, title, detail, fix string) checks.Finding {
	return checks.Finding{ID: id, Title: title, Status: checks.StatusWarn, Severity: sev, Detail: detail, Fix: fix}
}

// Findings turns measurements into plain-language problems, most useful first.
// Only problems are reported; a healthy measurement adds nothing.
func Findings(r *Report) []checks.Finding {
	var s []checks.Finding
	if w := r.WiFi; w != nil {
		if w.Band == "2.4GHz" && w.Supports5GHz {
			s = append(s, warn("wifi-band", "high", "Use 5 GHz Wi-Fi",
				"You're on 2.4 GHz, which is slower and shared with neighbours, microwaves and Bluetooth.",
				"If your router has a second network (often named with '5G'), connect to it. If both bands share one name, enable 'band steering' in the router."))
		}
		if q := SignalQuality(w.SignalDBm); q == "weak" || q == "fair" {
			sev := "medium"
			if q == "weak" {
				sev = "high"
			}
			s = append(s, warn("wifi-signal", sev, "Improve Wi-Fi signal",
				fmt.Sprintf("Signal is %s (%d dBm). Below about -67 dBm, speed drops and calls stutter.", q, w.SignalDBm),
				"Move the router to a central, raised spot away from walls and metal, or add a mesh node / access point near where you work."))
		}
	}
	if c := r.Congestion; c != nil && c.BestChannel != 0 && c.OnMyChannel-c.BestCount >= 2 {
		s = append(s, warn("wifi-channel", "medium", fmt.Sprintf("Switch Wi-Fi channel to %d", c.BestChannel),
			fmt.Sprintf("%d nearby networks share your channel's airtime; channel %d has %d.", c.OnMyChannel, c.BestChannel, c.BestCount),
			fmt.Sprintf("In the router admin page, set the %s channel to %d instead of 'Auto' (usually under Wireless settings).", c.Band, c.BestChannel)))
	}
	if g := r.Gateway; g != nil && g.Received > 0 && (g.JitterMs > 10 || g.MedianMs > 20 || g.LossPct > 2) {
		s = append(s, warn("gateway-latency", "medium", "Your Wi-Fi link to the router is unstable",
			fmt.Sprintf("Router ping: median %.1f ms, jitter %.1f ms, %.0f%% loss. A healthy link is under 5 ms with little jitter.", g.MedianMs, g.JitterMs, g.LossPct),
			"Usually interference or distance: try 5 GHz, a quieter channel, or a cable for your desk."))
	}
	if f, ok := dnsFinding(r.DNS); ok {
		s = append(s, f)
	}
	if b := r.Bufferbloat; b != nil && b.IncreaseMs >= 60 {
		s = append(s, warn("bufferbloat", "high", "Fix bufferbloat (lag when someone downloads)",
			fmt.Sprintf("Latency went up by %.0f ms during a download (grade %s). Video calls and games lag whenever the line is busy.", b.IncreaseMs, b.Grade),
			"Turn on SQM, Smart Queue, 'QoS' or 'Adaptive QoS' in the router if it has it, set slightly below your plan speed."))
	}
	return s
}

func dnsFinding(results []DNSResult) (checks.Finding, bool) {
	var sys, best *DNSResult
	for i := range results {
		d := &results[i]
		if d.System {
			sys = d
		} else if d.Failures < d.Queries && (best == nil || d.MedianMs < best.MedianMs) {
			best = d
		}
	}
	if sys == nil || best == nil {
		return checks.Finding{}, false
	}
	sysBroken := sys.Failures == sys.Queries
	if !sysBroken && (sys.MedianMs-best.MedianMs < 15 || best.MedianMs >= sys.MedianMs*0.7) {
		return checks.Finding{}, false
	}
	why := fmt.Sprintf("Your DNS (%s) takes %.0f ms per lookup; %s (%s) takes %.0f ms. Every new website pays this.", sys.Server, sys.MedianMs, best.Name, best.Server, best.MedianMs)
	if sysBroken {
		why = fmt.Sprintf("Your DNS (%s) did not answer our test lookups; %s (%s) answered in %.0f ms.", sys.Server, best.Name, best.Server, best.MedianMs)
	}
	f := warn("dns-speed", "medium", "Change DNS to "+best.Name+" ("+best.Server+")", why,
		"Set it once in the router's DHCP/DNS settings so every device benefits, or per device in network settings.")
	for _, p := range PublicResolvers {
		if p.IP == best.Server {
			f.AutoFix = &checks.AutoFix{
				ID:      "set-dns",
				Summary: fmt.Sprintf("Switch this computer's DNS to %s (%s, %s). Other devices are not changed.", p.Name, p.IP, p.Secondary),
				Args:    []string{p.IP, p.Secondary},
			}
		}
	}
	return f, true
}
