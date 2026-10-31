package diagnose

import (
	"context"
	"fmt"
	"time"

	"github.com/Kavin-bm/fixmyrouter/internal/discover"
	"github.com/Kavin-bm/fixmyrouter/internal/fix"
)

// MissingTest returns the flag needed to measure metric, if the options
// don't include that test (the download tests need consent).
func MissingTest(metric string, opt Options) string {
	switch {
	case metric == MetricLoaded && !opt.Load:
		return "--load"
	case metric == MetricDownload && !opt.Throughput && !opt.Load:
		return "--throughput"
	}
	return ""
}

// PlanAuto previews a LevelAuto action on this computer. Nothing changes.
func PlanAuto(a *Action) (*fix.Change, error) {
	if a == nil || a.Level != LevelAuto || a.AutoFix == nil {
		return nil, fmt.Errorf("not an automatic fix")
	}
	_, iface, err := discover.DefaultGateway()
	if err != nil {
		return nil, err
	}
	return fix.Plan(a.AutoFix, iface)
}

// Remeasure diagnoses again with the same options and compares to before.
func Remeasure(ctx context.Context, p Prober, version string, before *Diagnosis, metric string, settle time.Duration) (*Diagnosis, Delta, error) {
	if settle > 0 {
		select {
		case <-time.After(settle):
		case <-ctx.Done():
			return nil, Delta{}, ctx.Err()
		}
	}
	after, err := Run(ctx, p, version, before.Options)
	if err != nil {
		return nil, Delta{}, err
	}
	return after, Compare(before, after, metric), nil
}

// Result is a measured fix: what was done and the before/after numbers.
type Result struct {
	Action   Action     `json:"action"`
	ChangeID int        `json:"change_id,omitempty"` // for `fixmyrouter undo`
	Before   *Diagnosis `json:"before"`
	After    *Diagnosis `json:"after"`
	Delta    Delta      `json:"delta"`
	Undo     string     `json:"undo,omitempty"`
}
