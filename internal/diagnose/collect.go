// Package diagnose names the main reason the internet feels slow (Wi-Fi,
// bufferbloat, DNS, the router, or the ISP), backs it with numbers, and maps
// each bottleneck to fixes: automatic on this computer, automatic on the
// router where a profile allows it, guided steps, or advice.
//
// It cannot make the ISP's line faster. It finds what is slowing you down
// below that line and fixes what can be fixed.
package diagnose

import (
	"context"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/Kavin-bm/fixmyrouter/internal/discover"
	"github.com/Kavin-bm/fixmyrouter/internal/speed"
)

// Options select the measurements. Load and Throughput download real data,
// so they only run when the person asked (flag or UI checkbox).
type Options struct {
	Load         bool    `json:"load"`                    // bufferbloat test, up to 200 MB
	Throughput   bool    `json:"throughput"`              // WAN download test, up to ThroughputMB
	ThroughputMB int     `json:"throughput_mb,omitempty"` // cap, default 25
	PlanMbps     float64 `json:"plan_mbps,omitempty"`     // the download speed the person pays for, if known
}

// WANTargets are pinged to measure the internet path beyond the router.
// Two providers, so one slow endpoint does not get blamed on the ISP.
var WANTargets = []string{"1.1.1.1", "8.8.8.8"}

// Prober does the actual measuring. Real uses the network; tests use fakes.
type Prober interface {
	Gateway(ctx context.Context) (*discover.Gateway, error)
	WiFi() (speed.WiFi, bool)
	Ping(ctx context.Context, target string, count int) (speed.PingStats, error)
	SystemResolvers() []string
	DNS(ctx context.Context, name, server string, rounds int) speed.DNSResult
	Bufferbloat(ctx context.Context) (speed.Bufferbloat, error)
	Throughput(ctx context.Context, capMB int) (speed.Throughput, error)
	GOOS() string
}

// Real measures this computer's network.
type Real struct{}

func (Real) Gateway(ctx context.Context) (*discover.Gateway, error) { return discover.Run(ctx, false) }
func (Real) WiFi() (speed.WiFi, bool)                               { return speed.DetectWiFi() }
func (Real) Ping(ctx context.Context, t string, n int) (speed.PingStats, error) {
	return speed.Ping(ctx, t, n)
}
func (Real) SystemResolvers() []string { return speed.SystemResolvers() }
func (Real) DNS(ctx context.Context, name, server string, rounds int) speed.DNSResult {
	return speed.MeasureDNS(ctx, name, server, rounds)
}
func (Real) Bufferbloat(ctx context.Context) (speed.Bufferbloat, error) {
	return speed.MeasureBufferbloat(ctx, "1.1.1.1", 10*time.Second)
}
func (Real) Throughput(ctx context.Context, capMB int) (speed.Throughput, error) {
	return speed.MeasureThroughput(ctx, &http.Client{}, speed.ThroughputURL, capMB, 12*time.Second)
}
func (Real) GOOS() string { return runtime.GOOS }

// Collect runs the measurements. Pings and DNS run in parallel; the two
// download tests run one after the other so they don't skew each other.
func Collect(ctx context.Context, p Prober, opt Options) (Measurements, error) {
	g, err := p.Gateway(ctx)
	if err != nil {
		return Measurements{}, err
	}
	m := Measurements{Gateway: g, Local: &speed.Report{GatewayIP: g.IP}}
	r := m.Local
	var wg sync.WaitGroup
	var mu sync.Mutex
	wg.Add(3)
	go func() {
		defer wg.Done()
		if w, ok := p.WiFi(); ok {
			r.WiFi = &w
			if c, ok := speed.Analyze(w); ok {
				r.Congestion = &c
			}
		} else {
			r.WiFiNote = "Not on Wi-Fi, or the OS did not report Wi-Fi details."
		}
	}()
	go func() {
		defer wg.Done()
		if s, _ := p.Ping(ctx, g.IP, 20); s.Received > 0 {
			r.Gateway = &s
		}
		for _, t := range WANTargets {
			s, _ := p.Ping(ctx, t, 10)
			mu.Lock()
			m.WAN.Pings = append(m.WAN.Pings, s)
			mu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		var dns []speed.DNSResult
		if sys := p.SystemResolvers(); len(sys) > 0 {
			d := p.DNS(ctx, "Your current DNS", sys[0], 3)
			d.System = true
			dns = append(dns, d)
		}
		for _, pr := range speed.PublicResolvers {
			dns = append(dns, p.DNS(ctx, pr.Name, pr.IP, 3))
		}
		r.DNS = dns
	}()
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return m, err
	}
	if opt.Load {
		if b, err := p.Bufferbloat(ctx); err != nil {
			r.BloatNote = "Bufferbloat test failed: " + err.Error()
		} else {
			r.Bufferbloat = &b
		}
	}
	if opt.Throughput {
		if t, err := p.Throughput(ctx, opt.ThroughputMB); err != nil {
			m.WAN.Note = "Download test failed: " + err.Error()
		} else {
			m.WAN.Throughput = &t
		}
	}
	return m, ctx.Err()
}
