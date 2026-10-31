// Package report renders audit results for humans (terminal) and agents (JSON).
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/kavinbm/fixmyrouter/internal/checks"
	"github.com/kavinbm/fixmyrouter/internal/discover"
)

// Audit is the full result of `fixmyrouter audit`.
type Audit struct {
	Tool        string            `json:"tool"`
	Version     string            `json:"version"`
	GeneratedAt time.Time         `json:"generated_at"`
	ReadOnly    bool              `json:"read_only"`
	Gateway     *discover.Gateway `json:"gateway"`
	Findings    []checks.Finding  `json:"findings"`
	Summary     map[string]int    `json:"summary"`
	ComingSoon  []string          `json:"coming_soon"`
}

// Summarize counts findings by status.
func Summarize(fs []checks.Finding) map[string]int {
	m := map[string]int{checks.StatusPass: 0, checks.StatusWarn: 0, checks.StatusFail: 0, checks.StatusError: 0}
	for _, f := range fs {
		m[f.Status]++
	}
	return m
}

// JSON writes the audit as indented JSON.
func JSON(w io.Writer, a *Audit) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(a)
}

var icons = map[string]string{
	checks.StatusPass: "✅ PASS", checks.StatusWarn: "⚠️  WARN", checks.StatusFail: "❌ FAIL", checks.StatusError: "❔ ERROR",
}

// Text writes a human-readable report.
func Text(w io.Writer, a *Audit) {
	GatewayText(w, a.Gateway)
	fmt.Fprintln(w, "\nAudit (read-only, nothing was changed):")
	for _, f := range a.Findings {
		fmt.Fprintf(w, "\n%s  %s [%s]\n   %s\n", icons[f.Status], f.Title, f.Severity, f.Detail)
		if f.Fix != "" {
			fmt.Fprintf(w, "   How to fix: %s\n", f.Fix)
		}
	}
	s := a.Summary
	fmt.Fprintf(w, "\nSummary: %d pass, %d warn, %d fail, %d error\n", s[checks.StatusPass], s[checks.StatusWarn], s[checks.StatusFail], s[checks.StatusError])
	fmt.Fprintln(w, "More checks coming soon: WPS, DNS hijack, WAN-side remote admin, firmware age, Wi-Fi security mode.")
}

// GatewayText prints discovery info.
func GatewayText(w io.Writer, g *discover.Gateway) {
	fmt.Fprintf(w, "Gateway:   %s", g.IP)
	if g.Interface != "" {
		fmt.Fprintf(w, " (via %s)", g.Interface)
	}
	fmt.Fprintln(w)
	switch {
	case g.MAC == "":
		fmt.Fprintln(w, "MAC:       unknown (not in ARP cache)")
	case g.MACLocallyAdmin:
		fmt.Fprintf(w, "MAC:       %s (locally administered, vendor can't be read from it)\n", g.MAC)
	default:
		fmt.Fprintf(w, "MAC:       %s\n", g.MAC)
	}
	if g.Vendor != "" {
		fmt.Fprintf(w, "Vendor:    %s (from %s)\n", g.Vendor, g.VendorSource)
	} else {
		fmt.Fprintln(w, "Vendor:    unknown (try --online to look up the MAC prefix)")
	}
	if g.HTTPStatus != 0 {
		fmt.Fprintf(w, "Admin web: HTTP %d", g.HTTPStatus)
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
		fmt.Fprintln(w, "Admin web: no answer on http://"+g.IP+"/")
	}
}
