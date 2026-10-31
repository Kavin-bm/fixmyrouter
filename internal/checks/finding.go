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

// Finding is the result of one check.
type Finding struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Severity string `json:"severity"` // info, low, medium, high
	Detail   string `json:"detail"`
	Fix      string `json:"fix,omitempty"`
}

// Check is one read-only audit step against the gateway IP.
type Check struct {
	ID  string
	Run func(ctx context.Context, gatewayIP string) Finding
}

// All returns the checks that ship today.
func All() []Check {
	return []Check{
		{ID: "http-admin", Run: HTTPAdmin},
		{ID: "telnet", Run: Telnet},
		{ID: "upnp", Run: UPnP},
	}
}

const dialTimeout = 3 * time.Second
