package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/kavinbm16/fixmyrouter/internal/checks"
	"github.com/kavinbm16/fixmyrouter/internal/discover"
	"github.com/kavinbm16/fixmyrouter/internal/speed"
)

func TestText(t *testing.T) {
	fs := []checks.Finding{
		{ID: "dns-speed", Title: "Change DNS", Status: checks.StatusWarn, Severity: "medium", Detail: "slow", Fix: "set it",
			AutoFix: &checks.AutoFix{ID: "set-dns", Summary: "Switch DNS."}},
		{ID: "telnet", Title: "Telnet", Status: checks.StatusPass, Severity: "info", Detail: "closed"},
	}
	a := &Audit{Gateway: &discover.Gateway{IP: "192.168.1.1"}, Findings: fs, Summary: Summarize(fs), Score: checks.Score(fs),
		Speed: &speed.Report{GatewayIP: "192.168.1.1", DNS: []speed.DNSResult{{Name: "Cloudflare", Server: "1.1.1.1", MedianMs: 9, Queries: 15}}}}
	prev := &Audit{Score: 50, GeneratedAt: time.Now()}
	var b bytes.Buffer
	Text(&b, a, prev)
	for _, want := range []string{"Score: 88/100", "1 pass, 1 warn", "fixmyrouter fix dns-speed", "Before/after: 50 -> 88", "did not answer ping", "Cloudflare"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %q in:\n%s", want, b.String())
		}
	}
}
