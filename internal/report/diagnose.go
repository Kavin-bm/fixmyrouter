package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/Kavin-bm/fixmyrouter/internal/diagnose"
)

var levelLabel = map[string]string{
	diagnose.LevelAuto:   "fixmyrouter can do this for you",
	diagnose.LevelRouter: "fixmyrouter can change this on your router",
	diagnose.LevelGuided: "step by step for your router",
	diagnose.LevelAdvice: "what to do",
}

// DiagnosisText prints the verdict, evidence and fixes.
func DiagnosisText(w io.Writer, d *diagnose.Diagnosis) {
	fmt.Fprintf(w, "\n%s\n", d.Verdict)
	if d.Profile != "" {
		fmt.Fprintf(w, "Router profile: %s\n", d.Profile)
	}
	for i, b := range d.Bottlenecks {
		marker := "Also"
		if b.Kind == d.Primary {
			marker = "MAIN BOTTLENECK"
		}
		fmt.Fprintf(w, "\n%s: %s (impact %d/100)\n  %s\n", marker, b.Name, b.Impact, b.Headline)
		if b.Explain != "" {
			fmt.Fprintf(w, "  %s\n", b.Explain)
		}
		fmt.Fprintln(w, "  Evidence:")
		for _, e := range b.Evidence {
			line := fmt.Sprintf("    - %s: %s", e.Name, e.Value)
			if e.Note != "" {
				line += " (" + e.Note + ")"
			}
			fmt.Fprintln(w, line)
		}
		if i == 0 || b.Kind == d.Primary {
			ActionsText(w, b.Actions)
		} else if len(b.Actions) > 0 {
			fmt.Fprintf(w, "  Fixes: %s\n", actionIDs(b.Actions))
		}
	}
	if d.Healthy {
		WANText(w, d)
	}
	for _, n := range d.Notes {
		fmt.Fprintf(w, "\nNote: %s\n", n)
	}
}

func actionIDs(a []diagnose.Action) string {
	var s []string
	for _, x := range a {
		s = append(s, x.ID)
	}
	return strings.Join(s, ", ")
}

// ActionsText prints fixes grouped by how automatic they are.
func ActionsText(w io.Writer, acts []diagnose.Action) {
	if len(acts) == 0 {
		return
	}
	fmt.Fprintln(w, "  How to fix it:")
	for _, a := range acts {
		fmt.Fprintf(w, "    [%s] %s  (%s, id: %s)\n", a.Level, a.Title, levelLabel[a.Level], a.ID)
		for _, s := range a.Steps {
			fmt.Fprintf(w, "        - %s\n", s)
		}
		if a.Level == diagnose.LevelAuto || a.Level == diagnose.LevelRouter {
			fmt.Fprintf(w, "        Run: fixmyrouter fix --measure %s\n", a.ID)
		} else if a.Command != "" {
			fmt.Fprintf(w, "        Then: %s\n", a.Command)
		}
	}
}

// WANText prints the internet numbers for a healthy result.
func WANText(w io.Writer, d *diagnose.Diagnosis) {
	if b := diagnose.BestWAN(d.Measurements.WAN.Pings); b != nil {
		fmt.Fprintf(w, "\nInternet: %.0f ms to %s, %.0f%% loss, %.0f ms jitter.\n", b.MedianMs, b.Target, b.LossPct, b.JitterMs)
	}
	if t := d.Measurements.WAN.Throughput; t != nil && t.DownloadMbps > 0 {
		fmt.Fprintf(w, "Download: %.0f Mbps (%.0f MB in %.1f s).\n", t.DownloadMbps, t.DownloadedMB, t.Seconds)
	}
}

// DeltaText prints a before/after table and the honest verdict.
func DeltaText(w io.Writer, dl diagnose.Delta) {
	fmt.Fprintf(w, "\n%-28s %10s %10s %10s\n", "Before / after", "before", "after", "change")
	for _, m := range dl.Metrics {
		mark := " "
		switch {
		case m.Better:
			mark = "better"
		case m.Worse:
			mark = "worse"
		}
		name := m.Name
		if m.Target {
			name = "* " + name
		}
		unit := m.Unit
		fmt.Fprintf(w, "%-28s %10s %10s %+9.1f%s  %s\n", name, num(m.Before)+unit, num(m.After)+unit, m.Change, unitShort(unit), mark)
	}
	if dl.Target != "" {
		fmt.Fprintln(w, "(* = what this fix should improve; changes smaller than normal measurement noise are not counted)")
	}
	fmt.Fprintf(w, "\n%s\n", dl.Verdict)
}

func num(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

func unitShort(u string) string {
	if u == "%" {
		return "pt"
	}
	return ""
}
