// Package report renders results for humans (terminal) and agents (JSON).
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kavinbm/fixmyrouter/internal/checks"
	"github.com/kavinbm/fixmyrouter/internal/discover"
	"github.com/kavinbm/fixmyrouter/internal/speed"
)

// Audit is the full result of one run (audit, speed or check).
type Audit struct {
	Tool        string            `json:"tool"`
	Version     string            `json:"version"`
	Kind        string            `json:"kind"`
	GeneratedAt time.Time         `json:"generated_at"`
	ReadOnly    bool              `json:"read_only"`
	Score       int               `json:"score"`
	Grade       string            `json:"grade"`
	Gateway     *discover.Gateway `json:"gateway"`
	Profile     string            `json:"router_profile,omitempty"` // matched profiles/ file
	Speed       *speed.Report     `json:"speed,omitempty"`
	Findings    []checks.Finding  `json:"findings"`
	Summary     map[string]int    `json:"summary"`
	ComingSoon  []string          `json:"coming_soon,omitempty"`
}

// Summarize counts findings by status.
func Summarize(fs []checks.Finding) map[string]int {
	m := map[string]int{checks.StatusPass: 0, checks.StatusWarn: 0, checks.StatusFail: 0, checks.StatusError: 0}
	for _, f := range fs {
		m[f.Status]++
	}
	return m
}

// JSON writes v as indented JSON.
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

var icons = map[string]string{
	checks.StatusPass: "✅ PASS", checks.StatusWarn: "⚠️  WARN", checks.StatusFail: "❌ FAIL", checks.StatusError: "❔ ERROR",
}

// Text writes a human-readable report. prev, if not nil, is the last run of
// the same kind, for a before/after line.
func Text(w io.Writer, a *Audit, prev *Audit) {
	GatewayText(w, a.Gateway)
	if a.Speed != nil {
		fmt.Fprintln(w)
		SpeedText(w, a.Speed)
	}
	fmt.Fprintln(w, "\nFindings (read-only, nothing was changed):")
	for _, f := range a.Findings {
		fmt.Fprintf(w, "\n%s  %s [%s]\n   %s\n", icons[f.Status], f.Title, f.Severity, f.Detail)
		if f.Fix != "" {
			fmt.Fprintf(w, "   How to fix: %s\n", f.Fix)
		}
		if f.RouterSteps != "" {
			fmt.Fprintf(w, "   %s\n", f.RouterSteps)
		}
		if f.AutoFix != nil {
			fmt.Fprintf(w, "   Can do it for you: `fixmyrouter fix %s` (%s Asks first; `fixmyrouter undo` reverts.)\n", f.ID, f.AutoFix.Summary)
		}
	}
	if len(a.Findings) == 0 {
		fmt.Fprintln(w, "\n✅ Nothing obvious to fix.")
	}
	s := a.Summary
	fmt.Fprintf(w, "\nScore: %d/100 (%s). %d pass, %d warn, %d fail, %d could not run.\n", a.Score, a.Grade, s[checks.StatusPass], s[checks.StatusWarn], s[checks.StatusFail], s[checks.StatusError])
	if prev != nil {
		fmt.Fprintf(w, "Before/after: %d -> %d since %s.\n", prev.Score, a.Score, prev.GeneratedAt.Local().Format("Jan 2 15:04"))
	}
	if len(a.ComingSoon) > 0 {
		fmt.Fprintf(w, "Coming soon: %s.\n", strings.Join(a.ComingSoon, ", "))
	}
}

// SpeedText writes the measurements of a speed run.
func SpeedText(w io.Writer, r *speed.Report) {
	if wf := r.WiFi; wf != nil {
		fmt.Fprintf(w, "Wi-Fi:      %s, channel %d", wf.Band, wf.Channel)
		if wf.WidthMHz > 0 {
			fmt.Fprintf(w, " (%d MHz)", wf.WidthMHz)
		}
		if wf.PHYMode != "" {
			fmt.Fprintf(w, ", %s", wf.PHYMode)
		}
		if wf.TxRateMbps > 0 {
			fmt.Fprintf(w, ", link rate %d Mbps", wf.TxRateMbps)
		}
		if wf.Security != "" {
			fmt.Fprintf(w, ", %s", strings.ToUpper(wf.Security))
		}
		fmt.Fprintln(w)
		if wf.SignalDBm != 0 {
			fmt.Fprintf(w, "Signal:     %d dBm (%s)", wf.SignalDBm, speed.SignalQuality(wf.SignalDBm))
			if wf.NoiseDBm != 0 {
				fmt.Fprintf(w, ", noise %d dBm, SNR %d dB", wf.NoiseDBm, wf.SignalDBm-wf.NoiseDBm)
			}
			fmt.Fprintln(w)
		}
	} else {
		fmt.Fprintln(w, "Wi-Fi:      "+r.WiFiNote)
	}
	if c := r.Congestion; c != nil {
		fmt.Fprintf(w, "Neighbours: %d other %s networks visible; %d overlap your channel; quietest standard channel is %d (%d overlapping)\n", c.Seen, c.Band, c.OnMyChannel, c.BestChannel, c.BestCount)
	} else if r.WiFi != nil {
		fmt.Fprintln(w, "Neighbours: no scan data from the OS")
	}
	if g := r.Gateway; g != nil {
		fmt.Fprintf(w, "Router:     ping %s median %.1f ms, jitter %.1f ms, loss %.0f%% (%d pings)\n", g.Target, g.MedianMs, g.JitterMs, g.LossPct, g.Sent)
	} else {
		fmt.Fprintf(w, "Router:     %s did not answer ping (some routers block it; this is not a problem by itself)\n", r.GatewayIP)
	}
	if len(r.DNS) > 0 {
		fmt.Fprintln(w, "DNS lookup time (median of popular sites):")
		for _, d := range r.DNS {
			if d.Failures == d.Queries {
				fmt.Fprintf(w, "  %-18s %-28s no answer\n", d.Name, d.Server)
				continue
			}
			fmt.Fprintf(w, "  %-18s %-28s %6.1f ms", d.Name, d.Server, d.MedianMs)
			if d.Failures > 0 {
				fmt.Fprintf(w, "  (%d/%d failed)", d.Failures, d.Queries)
			}
			fmt.Fprintln(w)
		}
	}
	if b := r.Bufferbloat; b != nil {
		fmt.Fprintf(w, "Bufferbloat: grade %s. Ping %s idle %.1f ms -> %.1f ms while downloading (+%.1f ms); download ~%.0f Mbps\n",
			b.Grade, b.Target, b.Idle.MedianMs, b.Loaded.MedianMs, b.IncreaseMs, b.DownloadMbps)
	} else if r.BloatNote != "" {
		fmt.Fprintln(w, "Bufferbloat: "+r.BloatNote)
	}
}

// GatewayText prints discovery info.
func GatewayText(w io.Writer, g *discover.Gateway) {
	fmt.Fprintf(w, "Gateway:    %s", g.IP)
	if g.Interface != "" {
		fmt.Fprintf(w, " (via %s)", g.Interface)
	}
	fmt.Fprintln(w)
	switch {
	case g.MAC == "":
		fmt.Fprintln(w, "MAC:        unknown (not in ARP cache)")
	case g.MACLocallyAdmin:
		fmt.Fprintf(w, "MAC:        %s (locally administered, vendor can't be read from it)\n", g.MAC)
	default:
		fmt.Fprintf(w, "MAC:        %s\n", g.MAC)
	}
	if g.Vendor != "" {
		fmt.Fprintf(w, "Vendor:     %s (from %s)\n", g.Vendor, g.VendorSource)
	} else {
		fmt.Fprintln(w, "Vendor:     unknown (try `fixmyrouter discover --online` to look up the MAC prefix)")
	}
	if g.HTTPStatus != 0 {
		fmt.Fprintf(w, "Admin web:  HTTP %d", g.HTTPStatus)
		if g.HTTPTitle != "" {
			fmt.Fprintf(w, ", title %q", g.HTTPTitle)
		}
		if g.HTTPServer != "" {
			fmt.Fprintf(w, ", server %q", g.HTTPServer)
		}
		if g.HTTPRedirectLocation != "" {
			fmt.Fprintf(w, ", redirects to %s", g.HTTPRedirectLocation)
		}
		fmt.Fprintln(w)
	} else {
		fmt.Fprintln(w, "Admin web:  no answer on http://"+g.IP+"/")
	}
}
