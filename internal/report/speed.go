package report

import (
	"fmt"
	"io"

	"github.com/kavinbm/fixmyrouter/internal/speed"
)

// SpeedText writes a human-readable speed report.
func SpeedText(w io.Writer, r *speed.Report) {
	fmt.Fprintln(w, "Speed report (read-only, nothing was changed)")
	fmt.Fprintln(w)
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
	fmt.Fprintln(w, "\nDNS lookup time (median of popular sites):")
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
	fmt.Fprintln(w)
	if b := r.Bufferbloat; b != nil {
		fmt.Fprintf(w, "Bufferbloat: grade %s. Ping %s idle %.1f ms -> %.1f ms while downloading (+%.1f ms); download ~%.0f Mbps\n",
			b.Grade, b.Target, b.Idle.MedianMs, b.Loaded.MedianMs, b.IncreaseMs, b.DownloadMbps)
	} else {
		fmt.Fprintln(w, "Bufferbloat: "+r.BloatNote)
	}
	fmt.Fprintln(w)
	if len(r.Suggestions) == 0 {
		fmt.Fprintln(w, "✅ Nothing obvious to fix from these measurements.")
	} else {
		fmt.Fprintln(w, "Suggestions:")
		for i, s := range r.Suggestions {
			fmt.Fprintf(w, "\n%d. %s [%s impact]\n   Why: %s\n   How: %s\n", i+1, s.Title, s.Impact, s.Why, s.How)
		}
	}
	fmt.Fprintln(w, "\nComing soon: applying these changes for you (with a yes and an undo), Wi-Fi scan on 6 GHz.")
}
