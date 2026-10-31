// Package run is the one entry point the CLI, the web UI and the MCP server
// share: discover the gateway, run the chosen checks, score the result.
package run

import (
	"context"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/Kavin-bm/fixmyrouter/internal/checks"
	"github.com/Kavin-bm/fixmyrouter/internal/discover"
	"github.com/Kavin-bm/fixmyrouter/internal/report"
	"github.com/Kavin-bm/fixmyrouter/internal/speed"
	"github.com/Kavin-bm/fixmyrouter/internal/store"
	"github.com/Kavin-bm/fixmyrouter/profiles"
)

// Kinds of run.
const (
	KindAudit = "audit" // security only; talks to the gateway, nothing on the internet
	KindSpeed = "speed" // Wi-Fi, router latency, DNS comparison (+ bufferbloat with Load)
	KindCheck = "check" // both
)

// Options select what to run.
type Options struct {
	Kind string
	Load bool // bufferbloat test, downloads up to 200 MB
}

const runsFile = "runs.json"
const keepRuns = 30

// Run discovers the gateway and runs the checks for opt.Kind.
func Run(ctx context.Context, version string, opt Options) (*report.Audit, error) {
	g, err := discover.Run(ctx, false)
	if err != nil {
		return nil, err
	}
	a := &report.Audit{
		Tool: "fixmyrouter", Version: version, Kind: opt.Kind, GeneratedAt: time.Now().UTC(),
		ReadOnly: true, Gateway: g,
		ComingSoon: []string{"wps", "wan-remote-admin", "firmware-age", "upload-bufferbloat", "router-side-fixes"},
	}
	security := opt.Kind != KindSpeed
	var mu sync.Mutex
	var wg sync.WaitGroup
	add := func(fs ...checks.Finding) {
		mu.Lock()
		a.Findings = append(a.Findings, fs...)
		mu.Unlock()
	}
	if security {
		for _, c := range checks.All() {
			wg.Add(1)
			go func() { defer wg.Done(); add(c.Run(ctx, g.IP)) }()
		}
	}
	if opt.Kind == KindAudit {
		if w, ok := speed.DetectWiFi(); ok {
			add(checks.WiFiSecurity(w.Security))
		}
	} else {
		s := speed.Run(ctx, g.IP, speed.Options{Internet: true, Load: opt.Load})
		a.Speed = s
		add(speed.Findings(s)...)
		if security && s.WiFi != nil {
			add(checks.WiFiSecurity(s.WiFi.Security))
		}
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p := profiles.Find(g.HTTPTitle, g.HTTPServer); p != nil {
		a.Profile = p.File
		for i, f := range a.Findings {
			if f.Status == checks.StatusWarn || f.Status == checks.StatusFail {
				a.Findings[i].RouterSteps = p.Steps(f.ID, f.Title)
			}
		}
	}
	sortFindings(a.Findings)
	a.Summary = report.Summarize(a.Findings)
	a.Score = checks.Score(a.Findings)
	a.Grade = checks.Grade(a.Score)
	return a, nil
}

var group = map[string]int{checks.StatusFail: 0, checks.StatusWarn: 0, checks.StatusError: 1, checks.StatusPass: 2}
var sevRank = map[string]int{"high": 0, "medium": 1, "low": 2, "info": 3}

// sortFindings puts problems first (worst severity, then fail before warn),
// then checks that could not run, then passes.
func sortFindings(fs []checks.Finding) {
	key := func(f checks.Finding) [3]int {
		failFirst := 1
		if f.Status == checks.StatusFail {
			failFirst = 0
		}
		return [3]int{group[f.Status], sevRank[f.Severity], failFirst}
	}
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := key(fs[i]), key(fs[j])
		if a != b {
			return slices.Compare(a[:], b[:]) < 0
		}
		return fs[i].ID < fs[j].ID
	})
}

// Save appends a run to the local history (newest last, last 30 kept).
func Save(a *report.Audit) error {
	runs, err := History()
	if err != nil {
		return err
	}
	runs = append(runs, *a)
	if len(runs) > keepRuns {
		runs = runs[len(runs)-keepRuns:]
	}
	return store.Save(runsFile, runs)
}

// History returns saved runs, oldest first.
func History() ([]report.Audit, error) {
	var runs []report.Audit
	err := store.Load(runsFile, &runs)
	return runs, err
}

// Previous returns the most recent saved run of the same kind before a, or nil.
func Previous(a *report.Audit) *report.Audit {
	runs, _ := History()
	for i := len(runs) - 1; i >= 0; i-- {
		if runs[i].Kind == a.Kind && runs[i].GeneratedAt.Before(a.GeneratedAt) {
			return &runs[i]
		}
	}
	return nil
}

// FindAutoFix returns the latest saved finding with this ID that offers an automatic fix.
func FindAutoFix(id string) (*checks.Finding, *report.Audit) {
	runs, _ := History()
	for i := len(runs) - 1; i >= 0; i-- {
		for _, f := range runs[i].Findings {
			if f.ID == id && f.AutoFix != nil {
				return &f, &runs[i]
			}
		}
	}
	return nil, nil
}
