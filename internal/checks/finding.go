// Package checks holds the read-only audit checks. One file per check.
// Rules for every check: talk only to the default gateway, never send
// credentials, never change anything.
package checks

import (
	"context"
	"time"
)

// Status of a finding.
const (
	StatusPass  = "pass"
	StatusWarn  = "warn"
	StatusFail  = "fail"
	StatusError = "error"
)

// Finding is the result of one check or measurement.
type Finding struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Severity string `json:"severity"` // info, low, medium, high
	Detail   string `json:"detail"`
	Fix      string `json:"fix,omitempty"`
	// RouterSteps are model-specific steps from a community profile, if one matches.
	RouterSteps string   `json:"router_steps,omitempty"`
	AutoFix     *AutoFix `json:"auto_fix,omitempty"`
}

// AutoFix is a change fixmyrouter can make for you, only after a yes, with
// a snapshot first so `fixmyrouter undo` can put it back.
type AutoFix struct {
	ID      string   `json:"id"`      // e.g. "set-dns"
	Summary string   `json:"summary"` // what will change, in plain words
	Args    []string `json:"args,omitempty"`
}

// Check is one read-only audit step against the gateway IP.
type Check struct {
	ID  string
	Run func(ctx context.Context, gatewayIP string) Finding
}

// All returns the network checks that ship today.
func All() []Check {
	return []Check{
		{ID: "http-admin", Run: HTTPAdmin},
		{ID: "telnet", Run: Telnet},
		{ID: "upnp", Run: UPnP},
	}
}

const dialTimeout = 3 * time.Second

var penalty = map[string]int{"high": 25, "medium": 12, "low": 5}

// Score turns findings into one 0-100 number: start at 100 and subtract per
// open problem by severity (high 25, medium 12, low 5). Passes and checks
// that could not run cost nothing.
func Score(fs []Finding) int {
	s := 100
	for _, f := range fs {
		if f.Status == StatusWarn || f.Status == StatusFail {
			s -= penalty[f.Severity]
		}
	}
	return max(s, 0)
}

// Grade puts a score into words.
func Grade(score int) string {
	switch {
	case score >= 90:
		return "Great"
	case score >= 75:
		return "Good"
	case score >= 50:
		return "Needs work"
	}
	return "At risk"
}
