// Package speed measures why home Wi-Fi feels slow. Everything is read-only:
// it reads what the OS already knows, pings your own gateway, times DNS
// lookups, and (only with --load) downloads test data to check bufferbloat.
package speed

import (
	"context"
	"fmt"
	"time"
)

// Suggestion is one plain-language thing to try.
type Suggestion struct {
	Title  string `json:"title"`
	Why    string `json:"why"`
	How    string `json:"how"`
	Impact string `json:"impact"` // high, medium, low
}

// Report is the full speed report.
type Report struct {
	GatewayIP   string       `json:"gateway_ip"`
	WiFi        *WiFi        `json:"wifi,omitempty"`
	WiFiNote    string       `json:"wifi_note,omitempty"`
	Congestion  *Congestion  `json:"congestion,omitempty"`
	Gateway     *PingStats   `json:"gateway_latency,omitempty"`
	DNS         []DNSResult  `json:"dns"`
	Bufferbloat *Bufferbloat `json:"bufferbloat,omitempty"`
	BloatNote   string       `json:"bufferbloat_note,omitempty"`
	Suggestions []Suggestion `json:"suggestions"`
}

// Options control optional, heavier measurements.
type Options struct {
	Load      bool // run the bufferbloat test (downloads up to 200 MB for ~10 s)
	DNSRounds int
}

// Run performs every measurement and builds suggestions.
func Run(ctx context.Context, gatewayIP string, opt Options) *Report {
	r := &Report{GatewayIP: gatewayIP}
	if w, ok := DetectWiFi(); ok {
		r.WiFi = &w
		if c, ok := Analyze(w); ok {
			r.Congestion = &c
		}
	} else {
		r.WiFiNote = "Not on Wi-Fi, or the OS did not report Wi-Fi details."
	}
	if p, err := Ping(ctx, gatewayIP, 20); err == nil || p.Received > 0 {
		r.Gateway = &p
	}
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
	if opt.Load {
		b, err := MeasureBufferbloat(ctx, "1.1.1.1", 10*time.Second)
		if err != nil {
			r.BloatNote = "Bufferbloat test failed: " + err.Error()
		} else {
			r.Bufferbloat = &b
		}
	} else {
		r.BloatNote = "Skipped. Run `fixmyrouter speed --load` to test latency under load (downloads up to 200 MB over ~10 s)."
	}
	r.Suggestions = Suggest(r)
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

// Suggest turns measurements into plain-language advice, most useful first.
func Suggest(r *Report) []Suggestion {
	var s []Suggestion
	if w := r.WiFi; w != nil {
		if w.Band == "2.4GHz" && w.Supports5GHz {
			s = append(s, Suggestion{
				Title:  "Use 5 GHz Wi-Fi",
				Why:    "You're on 2.4 GHz, which is slower and shared with neighbours, microwaves and Bluetooth.",
				How:    "If your router has a second network (often named with '5G'), connect to it. If both bands share one name, enable 'band steering' in the router.",
				Impact: "high",
			})
		}
		if q := SignalQuality(w.SignalDBm); q == "weak" || q == "fair" {
			s = append(s, Suggestion{
				Title:  "Improve Wi-Fi signal",
				Why:    fmt.Sprintf("Signal is %s (%d dBm). Below about -67 dBm, speed drops and calls stutter.", q, w.SignalDBm),
				How:    "Move the router to a central, raised spot away from walls and metal, or add a mesh node / access point near where you work.",
				Impact: "high",
			})
		}
	}
	if c := r.Congestion; c != nil && c.BestChannel != 0 && c.OnMyChannel-c.BestCount >= 2 {
		s = append(s, Suggestion{
			Title:  fmt.Sprintf("Switch Wi-Fi channel to %d", c.BestChannel),
			Why:    fmt.Sprintf("%d nearby networks share your channel's airtime; channel %d has %d.", c.OnMyChannel, c.BestChannel, c.BestCount),
			How:    fmt.Sprintf("In the router admin page, set the %s channel to %d instead of 'Auto' (usually under Wireless settings).", c.Band, c.BestChannel),
			Impact: "medium",
		})
	}
	if g := r.Gateway; g != nil && g.Received > 0 && (g.JitterMs > 10 || g.MedianMs > 20 || g.LossPct > 2) {
		s = append(s, Suggestion{
			Title:  "Your Wi-Fi link to the router is unstable",
			Why:    fmt.Sprintf("Router ping: median %.1f ms, jitter %.1f ms, %.0f%% loss. A healthy link is under 5 ms with little jitter.", g.MedianMs, g.JitterMs, g.LossPct),
			How:    "Usually interference or distance: try 5 GHz, a quieter channel, or a cable for your desk.",
			Impact: "medium",
		})
	}
	var sys *DNSResult
	var best *DNSResult
	for i := range r.DNS {
		d := &r.DNS[i]
		if d.System {
			sys = d
			continue
		}
		if d.Failures < d.Queries && (best == nil || d.MedianMs < best.MedianMs) {
			best = d
		}
	}
	if sys != nil && best != nil {
		sysBroken := sys.Failures == sys.Queries
		if sysBroken || (sys.MedianMs-best.MedianMs >= 15 && best.MedianMs < sys.MedianMs*0.7) {
			why := fmt.Sprintf("Your DNS (%s) takes %.0f ms per lookup; %s (%s) takes %.0f ms. Every new website pays this.", sys.Server, sys.MedianMs, best.Name, best.Server, best.MedianMs)
			if sysBroken {
				why = fmt.Sprintf("Your DNS (%s) did not answer our test lookups; %s (%s) answered in %.0f ms.", sys.Server, best.Name, best.Server, best.MedianMs)
			}
			s = append(s, Suggestion{
				Title:  "Change DNS to " + best.Name + " (" + best.Server + ")",
				Why:    why,
				How:    "Set it once in the router's DHCP/DNS settings so every device benefits, or per device in network settings.",
				Impact: "medium",
			})
		}
	}
	if b := r.Bufferbloat; b != nil && b.IncreaseMs >= 60 {
		s = append(s, Suggestion{
			Title:  "Fix bufferbloat (lag when someone downloads)",
			Why:    fmt.Sprintf("Latency went up by %.0f ms during a download (grade %s). Video calls and games lag whenever the line is busy.", b.IncreaseMs, b.Grade),
			How:    "Turn on SQM, Smart Queue, 'QoS' or 'Adaptive QoS' in the router if it has it, set slightly below your plan speed.",
			Impact: "high",
		})
	}
	return s
}
