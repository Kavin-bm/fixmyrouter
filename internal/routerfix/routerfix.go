// Package routerfix changes a setting on the router itself, only when a
// community profile names a built-in driver for that setting and the person
// says yes. Rules, enforced here and in the CLI:
//
//   - Only the default gateway (a private LAN address) is ever contacted.
//   - The person types the admin password for every session. It is sent once
//     (no retries, no guessing, no defaults) and never stored or logged.
//   - The current values are saved as a snapshot before anything changes, and
//     `fixmyrouter router undo` writes them back.
//   - Only the settings in RouterDrivers can change: Wi-Fi channel and SQM.
package routerfix

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/Kavin-bm/fixmyrouter/internal/store"
	"github.com/Kavin-bm/fixmyrouter/profiles"
)

const changesFile = "router_changes.json"

// Change statuses.
const (
	StatusPending = "pending"
	StatusApplied = "applied"
	StatusFailed  = "failed"
	StatusUndone  = "undone"
)

// Edit is one option on the router: its value before and after.
type Edit struct {
	Config    string `json:"config"`
	Section   string `json:"section"`
	Option    string `json:"option"`
	Before    string `json:"before"`
	BeforeSet bool   `json:"before_set"` // false: the option did not exist, undo deletes it
	After     string `json:"after"`
}

// Change is one router-side change and the snapshot needed to undo it.
type Change struct {
	ID        int        `json:"id"`
	Driver    string     `json:"driver"`
	Profile   string     `json:"profile"`
	Router    string     `json:"router"` // gateway IP
	Setting   string     `json:"setting"`
	Value     string     `json:"value"`
	Summary   string     `json:"summary"`
	Edits     []Edit     `json:"edits"`
	Status    string     `json:"status"`
	Error     string     `json:"error,omitempty"`
	AppliedAt time.Time  `json:"applied_at"`
	UndoneAt  *time.Time `json:"undone_at,omitempty"`
}

// Session is a logged-in connection to the router's settings.
type Session interface {
	// Sections returns every section of a config: name -> option -> value.
	// Option ".type" holds the section type.
	Sections(ctx context.Context, config string) (map[string]map[string]string, error)
	Get(ctx context.Context, config, section, option string) (value string, ok bool, err error)
	Set(ctx context.Context, config, section, option, value string) error
	Delete(ctx context.Context, config, section, option string) error
	Commit(ctx context.Context, config string) error // saves and reloads the affected service
}

// Driver logs in to a router model family.
type Driver interface {
	Name() string
	Login(ctx context.Context, baseURL, user, password string) (Session, error)
	Plan(ctx context.Context, s Session, setting, value string) (summary string, edits []Edit, err error)
}

var drivers = map[string]Driver{}

func register(d Driver) { drivers[d.Name()] = d }

// ErrNotAllowed means the profile does not allow this router-side change.
var ErrNotAllowed = errors.New("no router profile allows changing this setting automatically; follow the guided steps instead")

// DriverFor returns the driver a profile allows for setting.
func DriverFor(p *profiles.Profile, setting string) (Driver, error) {
	if !p.CanRouterFix(setting) {
		return nil, ErrNotAllowed
	}
	d, ok := drivers[p.RouterFix.Driver]
	if !ok {
		return nil, fmt.Errorf("driver %q is not built in", p.RouterFix.Driver)
	}
	return d, nil
}

// BaseURL returns the admin URL for the gateway. It refuses anything that is
// not a private LAN address, so a profile or bug can never point it elsewhere.
func BaseURL(gatewayIP string, https bool) (string, error) {
	ip := net.ParseIP(gatewayIP)
	if ip == nil || !(ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
		return "", fmt.Errorf("refusing to contact %q: router changes only go to your own private gateway", gatewayIP)
	}
	scheme := "http"
	if https {
		scheme = "https"
	}
	return scheme + "://" + net.JoinHostPort(gatewayIP, map[bool]string{true: "443", false: "80"}[https]), nil
}

// Plan reads the current values and returns the change, without changing anything.
func Plan(ctx context.Context, d Driver, s Session, p *profiles.Profile, gatewayIP, setting, value string) (*Change, error) {
	summary, edits, err := d.Plan(ctx, s, setting, value)
	if err != nil {
		return nil, err
	}
	return &Change{Driver: d.Name(), Profile: p.File, Router: gatewayIP, Setting: setting, Value: value, Summary: summary, Edits: edits}, nil
}

// Apply saves the snapshot, then writes and commits each edit and reads it back.
func Apply(ctx context.Context, s Session, c *Change) error {
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
	err := write(ctx, s, c.Edits, false)
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

// Pending returns the change Undo(id) would revert, so the CLI can name the
// router before asking for the password.
func Pending(id int) (*Change, error) {
	all, err := History()
	if err != nil {
		return nil, err
	}
	for j := len(all) - 1; j >= 0; j-- {
		if (id == 0 || all[j].ID == id) && all[j].Status != StatusUndone {
			return &all[j], nil
		}
	}
	return nil, fmt.Errorf("no router change to undo")
}

// Undo writes the snapshot of change id (latest if 0) back to the router.
func Undo(ctx context.Context, s Session, id int) (*Change, error) {
	var all []Change
	if err := store.Load(changesFile, &all); err != nil {
		return nil, err
	}
	i := -1
	for j := len(all) - 1; j >= 0; j-- {
		if (id == 0 || all[j].ID == id) && all[j].Status != StatusUndone {
			i = j
			break
		}
	}
	if i < 0 {
		return nil, fmt.Errorf("no router change to undo")
	}
	c := &all[i]
	if err := write(ctx, s, c.Edits, true); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	c.Status, c.UndoneAt = StatusUndone, &now
	return c, store.Save(changesFile, all)
}

// History returns every router change, oldest first.
func History() ([]Change, error) {
	var all []Change
	err := store.Load(changesFile, &all)
	return all, err
}

func write(ctx context.Context, s Session, edits []Edit, undo bool) error {
	var configs []string
	for _, e := range edits {
		var err error
		want, set := e.After, true
		if undo {
			want, set = e.Before, e.BeforeSet
		}
		if set {
			err = s.Set(ctx, e.Config, e.Section, e.Option, want)
		} else {
			err = s.Delete(ctx, e.Config, e.Section, e.Option)
		}
		if err != nil {
			return fmt.Errorf("%s.%s.%s: %w", e.Config, e.Section, e.Option, err)
		}
		if !containsStr(configs, e.Config) {
			configs = append(configs, e.Config)
		}
	}
	for _, cfg := range configs {
		if err := s.Commit(ctx, cfg); err != nil {
			return fmt.Errorf("saving %s: %w", cfg, err)
		}
	}
	for _, e := range edits { // read back: the router must report what we wrote
		want, set := e.After, true
		if undo {
			want, set = e.Before, e.BeforeSet
		}
		got, ok, err := s.Get(ctx, e.Config, e.Section, e.Option)
		if err != nil {
			return err
		}
		if ok != set || (set && got != want) {
			return fmt.Errorf("%s.%s.%s is %q after the change, expected %q", e.Config, e.Section, e.Option, got, want)
		}
	}
	return nil
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Describe lists the edits in plain words.
func Describe(edits []Edit) []string {
	var out []string
	for _, e := range edits {
		before := e.Before
		if !e.BeforeSet {
			before = "(not set)"
		}
		out = append(out, fmt.Sprintf("%s.%s.%s: %s -> %s", e.Config, e.Section, e.Option, before, e.After))
	}
	return out
}

// HasDriver reports whether name is a built-in driver (for tests and docs).
func HasDriver(name string) bool { _, ok := drivers[name]; return ok }

// trimKbit formats Mbps as the kbit/s integer OpenWrt SQM expects.
func trimKbit(mbps float64) string {
	return strings.TrimSuffix(fmt.Sprintf("%.0f", mbps*1000), ".0")
}
