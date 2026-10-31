package diagnose

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

// MaxBeforeAge is how old a saved "before" diagnosis may be for MeasureFix.
const MaxBeforeAge = 30 * time.Minute

// ErrRouterNeedsTerminal: router changes need the admin password typed by the
// person in the terminal, so agents and the web UI never handle it.
var ErrRouterNeedsTerminal = errors.New("router changes need the router admin password typed by you in a terminal: run `fixmyrouter fix --measure <id>` there")

// MeasureFix applies or confirms one fix from before and measures again.
// Auto fixes are applied here (confirmed must be true: the person said yes).
// Guided and advice fixes are done by the person; confirmed means they say
// they finished the steps. Router fixes are refused (ErrRouterNeedsTerminal).
func MeasureFix(ctx context.Context, p Prober, version string, before *Diagnosis, id string, confirmed bool, settle time.Duration) (*Result, error) {
	if before == nil {
		return nil, errors.New("no diagnosis yet: run diagnose first")
	}
	if age := time.Since(before.GeneratedAt); age > MaxBeforeAge {
		return nil, fmt.Errorf("the last diagnosis is %s old: run diagnose again so the before numbers are fresh", age.Round(time.Minute))
	}
	act, _ := before.FindAction(id)
	if act == nil {
		return nil, fmt.Errorf("fix %q is not in the last diagnosis", id)
	}
	if act.Metric == "" {
		return nil, fmt.Errorf("fix %q has no before/after measurement; %s", id, act.Command)
	}
	if f := MissingTest(act.Metric, before.Options); f != "" {
		return nil, fmt.Errorf("to measure %q, diagnose again with the %s test first (it downloads data, so ask the person)", id, strings.TrimPrefix(f, "--"))
	}
	if act.Level == LevelRouter {
		return nil, ErrRouterNeedsTerminal
	}
	if !confirmed {
		if act.Level == LevelAuto {
			return nil, errors.New("refused: show the person the change and get an explicit yes, then call again with confirmation")
		}
		return nil, errors.New("waiting: the person should do the steps first, then confirm they are done")
	}
	res := &Result{Action: *act, Before: before}
	if act.Level == LevelAuto {
		c, err := PlanAuto(act)
		if err != nil {
			return nil, err
		}
		if err := fix.Apply(c); err != nil {
			return nil, fmt.Errorf("change %d failed: %w (undo %d to be sure the old setting is back)", c.ID, err, c.ID)
		}
		res.ChangeID, res.Undo = c.ID, fmt.Sprintf("fixmyrouter undo %d", c.ID)
	} else {
		settle = 0
	}
	after, dl, err := Remeasure(ctx, p, version, before, act.Metric, settle)
	if err != nil {
		return res, err
	}
	_ = Save(after)
	res.After, res.Delta = after, dl
	return res, nil
}
