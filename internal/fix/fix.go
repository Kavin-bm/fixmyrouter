// Package fix applies opt-in changes and undoes them. Every change is
// snapshotted to ~/.fixmyrouter/changes.json before anything is touched.
//
// Today the only automatic fix is switching this computer's DNS servers on
// macOS. Router settings are never changed: fixmyrouter never logs in to the
// router, so router fixes are step-by-step instructions instead.
package fix

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/kavinbm16/fixmyrouter/internal/checks"
	"github.com/kavinbm16/fixmyrouter/internal/store"
)

const changesFile = "changes.json"

// Change statuses.
const (
	StatusPending = "pending" // snapshot saved, change not confirmed yet
	StatusApplied = "applied"
	StatusFailed  = "failed"
	StatusUndone  = "undone"
)

// Change is one applied fix and the snapshot needed to undo it.
type Change struct {
	ID        int        `json:"id"`
	FixID     string     `json:"fix_id"`
	Summary   string     `json:"summary"`
	Service   string     `json:"service"` // macOS network service, e.g. "Wi-Fi"
	Before    []string   `json:"before"`  // DNS servers before; empty = automatic (from the router)
	After     []string   `json:"after"`
	Status    string     `json:"status"`
	Error     string     `json:"error,omitempty"`
	AppliedAt time.Time  `json:"applied_at"`
	UndoneAt  *time.Time `json:"undone_at,omitempty"`
}

// ErrUnsupported means this fix cannot be applied automatically here.
var ErrUnsupported = errors.New("automatic DNS change is only available on macOS for now; follow the manual steps in the finding")

// run executes a command and returns combined output. Replaced in tests.
var run = func(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

var goos = runtime.GOOS

// Plan validates an auto-fix and reads the current setting, without
// changing anything. iface is the interface of the default route (e.g. en0).
func Plan(a *checks.AutoFix, iface string) (*Change, error) {
	if a == nil || a.ID != "set-dns" {
		return nil, fmt.Errorf("unknown fix")
	}
	if goos != "darwin" {
		return nil, ErrUnsupported
	}
	if len(a.Args) == 0 || len(a.Args) > 3 {
		return nil, fmt.Errorf("set-dns needs 1-3 server addresses")
	}
	for _, ip := range a.Args {
		if net.ParseIP(ip) == nil {
			return nil, fmt.Errorf("not an IP address: %q", ip)
		}
	}
	out, err := run("networksetup", "-listnetworkserviceorder")
	if err != nil {
		return nil, err
	}
	svc := ServiceForDevice(out, iface)
	if svc == "" {
		return nil, fmt.Errorf("no network service found for interface %q", iface)
	}
	before, err := currentDNS(svc)
	if err != nil {
		return nil, err
	}
	return &Change{FixID: a.ID, Summary: a.Summary, Service: svc, Before: before, After: a.Args}, nil
}

// Apply saves the snapshot first, then makes the change and checks it took.
func Apply(c *Change) error {
	var all []Change
	if err := store.Load(changesFile, &all); err != nil {
		return fmt.Errorf("reading snapshots: %w", err)
	}
	c.ID = len(all) + 1
	c.Status, c.AppliedAt = StatusPending, time.Now().UTC()
	all = append(all, *c)
	if err := store.Save(changesFile, all); err != nil {
		return fmt.Errorf("saving snapshot (nothing was changed): %w", err)
	}
	err := setDNS(c.Service, c.After)
	c.Status = StatusApplied
	if err != nil {
		c.Status, c.Error = StatusFailed, err.Error()
	}
	all[len(all)-1] = *c
	if serr := store.Save(changesFile, all); serr != nil && err == nil {
		err = serr
	}
	return err
}

// Undo restores the snapshot of change id, or the latest applied change if id is 0.
func Undo(id int) (*Change, error) {
	var all []Change
	if err := store.Load(changesFile, &all); err != nil {
		return nil, err
	}
	i := -1
	for j := len(all) - 1; j >= 0; j-- {
		// Failed changes are undoable too: restoring the snapshot is harmless
		// if nothing changed, and needed if the change half-applied.
		if (id == 0 || all[j].ID == id) && all[j].Status != StatusUndone {
			i = j
			break
		}
	}
	if i < 0 {
		return nil, fmt.Errorf("nothing to undo")
	}
	c := &all[i]
	if err := setDNS(c.Service, c.Before); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	c.Status, c.UndoneAt = StatusUndone, &now
	return c, store.Save(changesFile, all)
}

// History returns every change, oldest first.
func History() ([]Change, error) {
	var all []Change
	err := store.Load(changesFile, &all)
	return all, err
}

func setDNS(svc string, servers []string) error {
	args := append([]string{"-setdnsservers", svc}, servers...)
	if len(servers) == 0 {
		args = append(args, "Empty") // back to automatic
	}
	if _, err := run("networksetup", args...); err != nil {
		return err
	}
	got, err := currentDNS(svc)
	if err != nil {
		return err
	}
	if !slices.Equal(got, servers) {
		return fmt.Errorf("DNS for %s is %v after the change, expected %v", svc, got, servers)
	}
	return nil
}

func currentDNS(svc string) ([]string, error) {
	out, err := run("networksetup", "-getdnsservers", svc)
	if err != nil {
		return nil, err
	}
	return ParseGetDNS(out), nil
}

// ParseGetDNS parses `networksetup -getdnsservers`. "There aren't any DNS
// Servers set on Wi-Fi." means automatic, returned as an empty list.
func ParseGetDNS(out string) []string {
	servers := []string{}
	for _, f := range strings.Fields(out) {
		if net.ParseIP(f) != nil {
			servers = append(servers, f)
		}
	}
	return servers
}

var serviceRe = regexp.MustCompile(`^\((?:\d+|\*)\)\s+(.+)$`)

// ServiceForDevice maps a device (en0) to its network service name ("Wi-Fi")
// using `networksetup -listnetworkserviceorder`.
func ServiceForDevice(out, device string) string {
	name := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if m := serviceRe.FindStringSubmatch(line); m != nil {
			name = strings.TrimPrefix(m[1], "*")
			continue
		}
		if strings.Contains(line, "Device: "+device+")") && name != "" {
			return name
		}
	}
	return ""
}
