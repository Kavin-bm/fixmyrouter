// Package isp collects timestamped evidence of slow internet for a
// complaint to the internet provider: repeated samples of the router link,
// the path beyond it, DNS and (optionally) download speed.
package isp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Kavin-bm/fixmyrouter/internal/diagnose"
	"github.com/Kavin-bm/fixmyrouter/internal/speed"
)

// Options for a report.
type Options struct {
	Samples      int           `json:"samples"`
	Interval     time.Duration `json:"interval_ns"`
	PlanMbps     float64       `json:"plan_mbps,omitempty"`
	ThroughputMB int           `json:"throughput_mb"`
	NoDownload   bool          `json:"no_download"`
}

// Limits keep a report from running away.
const (
	MaxSamples  = 48
	MinInterval = 10 * time.Second
)

// Validate checks and fills defaults.
func (o *Options) Validate() error {
	if o.Samples < 1 || o.Samples > MaxSamples {
		return fmt.Errorf("samples must be 1-%d", MaxSamples)
	}
	if o.Samples > 1 && o.Interval < MinInterval {
		return fmt.Errorf("interval must be at least %s", MinInterval)
	}
	if o.ThroughputMB <= 0 {
		o.ThroughputMB = speed.DefaultThroughputMB
	}
	if o.ThroughputMB > speed.MaxThroughputMB {
		o.ThroughputMB = speed.MaxThroughputMB
	}
	return nil
}

// DataBudgetMB is the most the report will download.
func (o Options) DataBudgetMB() int {
	if o.NoDownload {
		return 0
	}
	return o.Samples * o.ThroughputMB
}

// Sample is one measurement round.
type Sample struct {
	At           time.Time        `json:"at"`
	Gateway      *speed.PingStats `json:"gateway,omitempty"`
	WAN          *speed.PingStats `json:"wan,omitempty"`
	DNSMs        float64          `json:"dns_ms,omitempty"`
	DownloadMbps float64          `json:"download_mbps,omitempty"`
	LocalOK      bool             `json:"local_ok"`
	Problems     []string         `json:"problems,omitempty"`
}

// Summary aggregates the samples.
type Summary struct {
	DownloadMinMbps    float64 `json:"download_min_mbps,omitempty"`
	DownloadMedianMbps float64 `json:"download_median_mbps,omitempty"`
	DownloadMaxMbps    float64 `json:"download_max_mbps,omitempty"`
	BelowPlan          int     `json:"samples_below_80pct_of_plan"`
	WANMedianMs        float64 `json:"internet_latency_median_ms"`
	WANLossAvgPct      float64 `json:"internet_loss_avg_pct"`
	GatewayMedianMs    float64 `json:"router_latency_median_ms"`
	LocalOKSamples     int     `json:"samples_with_healthy_home_network"`
	ProblemSamples     int     `json:"samples_with_internet_problems"`
}

// Report is the ISP evidence report.
type Report struct {
	Tool       string    `json:"tool"`
	Version    string    `json:"version"`
	Started    time.Time `json:"started"`
	Finished   time.Time `json:"finished"`
	GatewayIP  string    `json:"gateway_ip"`
	Hotspot    bool      `json:"hotspot"`
	Options    Options   `json:"options"`
	Samples    []Sample  `json:"samples"`
	Summary    Summary   `json:"summary"`
	Conclusion string    `json:"conclusion"`
}

// Sleep waits d or until ctx ends. Tests replace it.
type Sleep func(ctx context.Context, d time.Duration) error

// RealSleep is the default Sleep.
func RealSleep(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Collect takes the samples. progress (may be nil) is called after each one.
// If ctx is cancelled after at least one sample, the partial report is returned.
func Collect(ctx context.Context, p diagnose.Prober, version string, o Options, sleep Sleep, progress func(i int, s Sample)) (*Report, error) {
	if err := o.Validate(); err != nil {
		return nil, err
	}
	g, err := p.Gateway(ctx)
	if err != nil {
		return nil, err
	}
	r := &Report{Tool: "fixmyrouter", Version: version, Started: time.Now().UTC(), GatewayIP: g.IP, Hotspot: diagnose.IsHotspot(g.IP), Options: o}
	for i := 0; i < o.Samples; i++ {
		if i > 0 {
			if err := sleep(ctx, o.Interval); err != nil {
				break
			}
		}
		s := sample(ctx, p, g.IP, o)
		if ctx.Err() != nil && i > 0 {
			break
		}
		r.Samples = append(r.Samples, s)
		if progress != nil {
			progress(i, s)
		}
	}
	r.Finished = time.Now().UTC()
	if len(r.Samples) == 0 {
		return nil, ctx.Err()
	}
	summarize(r)
	return r, nil
}

func sample(ctx context.Context, p diagnose.Prober, gw string, o Options) Sample {
	s := Sample{At: time.Now().UTC()}
	if g, _ := p.Ping(ctx, gw, 10); g.Received > 0 {
		s.Gateway = &g
	}
	var pings []speed.PingStats
	for _, t := range diagnose.WANTargets {
		w, _ := p.Ping(ctx, t, 10)
		pings = append(pings, w)
	}
	if b := diagnose.BestWAN(pings); b != nil {
		c := *b
		s.WAN = &c
	}
	if sys := p.SystemResolvers(); len(sys) > 0 {
		if d := p.DNS(ctx, "system", sys[0], 2); d.Queries > d.Failures {
			s.DNSMs = d.MedianMs
		}
	}
	if !o.NoDownload {
		if t, err := p.Throughput(ctx, o.ThroughputMB); err == nil {
			s.DownloadMbps = t.DownloadMbps
		} else {
			s.Problems = append(s.Problems, "download test failed: "+err.Error())
		}
	}
	s.LocalOK = s.Gateway != nil && s.Gateway.LossPct <= 2 && s.Gateway.MedianMs <= 20 && s.Gateway.JitterMs <= 10
	switch {
	case s.WAN == nil:
		s.Problems = append(s.Problems, "no reply from the internet")
	default:
		if s.WAN.LossPct > 2 {
			s.Problems = append(s.Problems, fmt.Sprintf("%.0f%% packet loss beyond the router", s.WAN.LossPct))
		}
		if s.Gateway != nil && s.WAN.MedianMs-s.Gateway.MedianMs > 80 {
			s.Problems = append(s.Problems, fmt.Sprintf("%.0f ms latency beyond the router", s.WAN.MedianMs-s.Gateway.MedianMs))
		}
	}
	if o.PlanMbps > 0 && s.DownloadMbps > 0 && s.DownloadMbps < 0.8*o.PlanMbps {
		s.Problems = append(s.Problems, fmt.Sprintf("download %.0f Mbps is %.0f%% of the %.0f Mbps plan", s.DownloadMbps, 100*s.DownloadMbps/o.PlanMbps, o.PlanMbps))
	}
	return s
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

func r1(v float64) float64 { return float64(int64(v*10+0.5)) / 10 }

func summarize(r *Report) {
	var dl, wan, gw []float64
	var loss float64
	n := 0
	sm := &r.Summary
	for _, s := range r.Samples {
		if s.DownloadMbps > 0 {
			dl = append(dl, s.DownloadMbps)
			if r.Options.PlanMbps > 0 && s.DownloadMbps < 0.8*r.Options.PlanMbps {
				sm.BelowPlan++
			}
		}
		if s.WAN != nil {
			wan = append(wan, s.WAN.MedianMs)
			loss += s.WAN.LossPct
			n++
		}
		if s.Gateway != nil {
			gw = append(gw, s.Gateway.MedianMs)
		}
		if s.LocalOK {
			sm.LocalOKSamples++
		}
		if len(s.Problems) > 0 {
			sm.ProblemSamples++
		}
	}
	if len(dl) > 0 {
		sort.Float64s(dl)
		sm.DownloadMinMbps, sm.DownloadMaxMbps, sm.DownloadMedianMbps = r1(dl[0]), r1(dl[len(dl)-1]), r1(median(dl))
	}
	sm.WANMedianMs, sm.GatewayMedianMs = r1(median(wan)), r1(median(gw))
	if n > 0 {
		sm.WANLossAvgPct = r1(loss / float64(n))
	}
	total := len(r.Samples)
	switch {
	case sm.ProblemSamples == 0:
		r.Conclusion = fmt.Sprintf("No internet-side problem in %d sample(s): the connection met the checks every time.", total)
	case sm.LocalOKSamples == total:
		r.Conclusion = fmt.Sprintf("Internet-side problems in %d of %d samples, while the home network (this device to the router) was healthy in every sample. This points to the connection beyond the router: the ISP's line or network.", sm.ProblemSamples, total)
	default:
		r.Conclusion = fmt.Sprintf("Internet-side problems in %d of %d samples, but the home network was also unhealthy in %d sample(s), so part of the slowdown may be local. Fix Wi-Fi/router issues first (`fixmyrouter diagnose`) for evidence the ISP can't dispute.", sm.ProblemSamples, total, total-sm.LocalOKSamples)
	}
	if r.Hotspot {
		r.Conclusion += " Measured over a phone hotspot, so the \"ISP\" here is the mobile network."
	}
}

// Markdown writes the human report.
func Markdown(w io.Writer, r *Report) {
	loc := time.Local
	fmt.Fprintf(w, "# Internet connection report\n\n")
	fmt.Fprintf(w, "Measured by fixmyrouter %s from %s to %s (%s), %d sample(s)", r.Version,
		r.Started.In(loc).Format("2006-01-02 15:04"), r.Finished.In(loc).Format("15:04"), r.Started.In(loc).Format("MST -07:00"), len(r.Samples))
	if len(r.Samples) > 1 {
		fmt.Fprintf(w, " every %s", r.Options.Interval)
	}
	fmt.Fprintf(w, ". Router (gateway): %s.\n\n", r.GatewayIP)
	if r.Options.PlanMbps > 0 {
		fmt.Fprintf(w, "Plan download speed: **%.0f Mbps**.\n\n", r.Options.PlanMbps)
	}
	fmt.Fprintf(w, "## Conclusion\n\n%s\n\n## Summary\n\n", r.Conclusion)
	s := r.Summary
	fmt.Fprintf(w, "| | |\n|---|---|\n")
	if s.DownloadMedianMbps > 0 {
		fmt.Fprintf(w, "| Download speed (min / median / max) | %.1f / %.1f / %.1f Mbps |\n", s.DownloadMinMbps, s.DownloadMedianMbps, s.DownloadMaxMbps)
	}
	if r.Options.PlanMbps > 0 {
		fmt.Fprintf(w, "| Samples below 80%% of plan | %d of %d |\n", s.BelowPlan, len(r.Samples))
	}
	fmt.Fprintf(w, "| Internet latency (median) | %.1f ms |\n| Internet packet loss (average) | %.1f %% |\n| Router latency (median) | %.1f ms |\n| Samples with a healthy home network | %d of %d |\n\n",
		s.WANMedianMs, s.WANLossAvgPct, s.GatewayMedianMs, s.LocalOKSamples, len(r.Samples))
	fmt.Fprintf(w, "## Samples\n\n| Time | Router ms (loss) | Internet ms (loss) | DNS ms | Download Mbps | Home network | Problems |\n|---|---|---|---|---|---|---|\n")
	for _, x := range r.Samples {
		gw, wan, dns, dl := "no reply", "no reply", "-", "-"
		if x.Gateway != nil {
			gw = fmt.Sprintf("%.1f (%.0f%%)", x.Gateway.MedianMs, x.Gateway.LossPct)
		}
		if x.WAN != nil {
			wan = fmt.Sprintf("%.1f (%.0f%%) %s", x.WAN.MedianMs, x.WAN.LossPct, x.WAN.Target)
		}
		if x.DNSMs > 0 {
			dns = fmt.Sprintf("%.0f", x.DNSMs)
		}
		if x.DownloadMbps > 0 {
			dl = fmt.Sprintf("%.1f", x.DownloadMbps)
		}
		local := "ok"
		if !x.LocalOK {
			local = "unhealthy"
		}
		probs := strings.Join(x.Problems, "; ")
		if probs == "" {
			probs = "none"
		}
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s | %s |\n", x.At.In(loc).Format("2006-01-02 15:04:05"), gw, wan, dns, dl, local, probs)
	}
	fmt.Fprintf(w, "\n## Method\n\n- Router: 10 pings to the default gateway. Internet: 10 pings each to %s (best shown).\n", strings.Join(diagnose.WANTargets, " and "))
	if !r.Options.NoDownload {
		fmt.Fprintf(w, "- Download: %d parallel HTTPS streams from speed.cloudflare.com, capped at %d MB per sample. One server, so it can read lower than your ISP's own test.\n", speed.ThroughputStreams, r.Options.ThroughputMB)
	}
	fmt.Fprintf(w, "- \"Home network healthy\" means router ping loss <= 2%%, median <= 20 ms, jitter <= 10 ms.\n- Raw numbers are in the JSON file next to this one.\n")
}

// Write saves the report as Markdown and JSON in dir, with a timestamped name.
func Write(dir string, r *Report) (md, js string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	base := filepath.Join(dir, "fixmyrouter-isp-report-"+r.Started.In(time.Local).Format("2006-01-02-1504"))
	md, js = base+".md", base+".json"
	var b strings.Builder
	Markdown(&b, r)
	if err := os.WriteFile(md, []byte(b.String()), 0o644); err != nil {
		return "", "", err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", "", err
	}
	return md, js, os.WriteFile(js, append(data, '\n'), 0o644)
}
