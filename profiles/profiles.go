// Package profiles loads the community router profiles in this directory:
// one YAML file per model saying where each setting lives in its admin page.
// They are embedded in the binary, so they work offline.
package profiles

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed */*.yaml
var embedded embed.FS

// Profile describes one router model. Never credentials: see README.md.
type Profile struct {
	Vendor          string             `yaml:"vendor" json:"vendor"`
	Model           string             `yaml:"model" json:"model"`
	HardwareVersion string             `yaml:"hardware_version,omitempty" json:"hardware_version,omitempty"`
	FirmwareTested  string             `yaml:"firmware_tested" json:"firmware_tested"`
	ISPBranded      bool               `yaml:"isp_branded" json:"isp_branded"`
	ContributedBy   string             `yaml:"contributed_by" json:"contributed_by"`
	Verified        bool               `yaml:"verified" json:"verified"` // menu paths tested on a real device
	Source          string             `yaml:"source,omitempty" json:"source,omitempty"`
	Match           Match              `yaml:"match" json:"match"`
	Admin           Admin              `yaml:"admin" json:"admin"`
	Settings        map[string]Setting `yaml:"settings" json:"settings"`
	// RouterFix, if set, lets `fixmyrouter router apply` change these settings
	// on the router itself, after a yes, with a snapshot and undo. Only drivers
	// built into fixmyrouter are allowed, and the person types the admin
	// password each time (never stored, never guessed).
	RouterFix *RouterFix `yaml:"router_fix,omitempty" json:"router_fix,omitempty"`
	File      string     `yaml:"-" json:"file"`
}

// RouterFix names a built-in driver and the settings it may change.
type RouterFix struct {
	Driver   string   `yaml:"driver" json:"driver"`
	Settings []string `yaml:"settings" json:"settings"`
}

// RouterDrivers lists the built-in router drivers and the settings each can
// change. Keep in sync with internal/routerfix and schema.json.
var RouterDrivers = map[string][]string{
	"openwrt-ubus": {"wifi_channel_2g", "wifi_channel_5g", "sqm_qos"},
}

// CanRouterFix reports whether this profile allows an automatic router-side
// change of setting.
func (p *Profile) CanRouterFix(setting string) bool {
	return p != nil && p.RouterFix != nil && contains(p.RouterFix.Settings, setting)
}

// Match says how to recognise the router from `fixmyrouter discover`.
type Match struct {
	HTTPTitleContains  []string `yaml:"http_title_contains" json:"http_title_contains"`
	HTTPServerContains []string `yaml:"http_server_contains" json:"http_server_contains"`
}

// Admin is the admin page address.
type Admin struct {
	URL            string `yaml:"url" json:"url"`
	HTTPSSupported bool   `yaml:"https_supported" json:"https_supported"`
}

// Setting is where one option lives. MenuPath "n/a" means the router has no such option.
type Setting struct {
	MenuPath string `yaml:"menu_path" json:"menu_path"`
	Notes    string `yaml:"notes,omitempty" json:"notes,omitempty"`
}

// Settings lists the allowed setting keys (keep in sync with schema.json).
var Settings = []string{
	"wifi_channel_2g", "wifi_channel_5g", "band_steering", "dns_servers", "sqm_qos",
	"telnet", "upnp", "https_admin", "wps", "remote_management", "wifi_security",
}

var secretRe = regexp.MustCompile(`(?i)\b(user(name)?|login|pass(word|wd)?|pwd)\s*[:=]\s*\S+`)

// Parse decodes and validates one profile. Unknown keys are errors.
func Parse(name string, data []byte) (Profile, error) {
	var p Profile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return p, fmt.Errorf("%s: %w", name, err)
	}
	p.File = name
	var errs []string
	need := func(v, field string) {
		if strings.TrimSpace(v) == "" {
			errs = append(errs, field+" is required")
		}
	}
	need(p.Vendor, "vendor")
	need(p.Model, "model")
	need(p.FirmwareTested, "firmware_tested")
	need(p.ContributedBy, "contributed_by")
	if len(p.Match.HTTPTitleContains)+len(p.Match.HTTPServerContains) == 0 {
		errs = append(errs, "match needs at least one http_title_contains or http_server_contains entry")
	}
	for _, m := range append(p.Match.HTTPTitleContains, p.Match.HTTPServerContains...) {
		if len(strings.TrimSpace(m)) < 3 {
			errs = append(errs, fmt.Sprintf("match entry %q is too short to be specific", m))
		}
	}
	if len(p.Settings) == 0 {
		errs = append(errs, "settings is empty")
	}
	for k, s := range p.Settings {
		if !contains(Settings, k) {
			errs = append(errs, fmt.Sprintf("unknown setting %q (allowed: %s)", k, strings.Join(Settings, ", ")))
		}
		need(s.MenuPath, "settings."+k+".menu_path")
	}
	if rf := p.RouterFix; rf != nil {
		supported, ok := RouterDrivers[rf.Driver]
		if !ok {
			errs = append(errs, fmt.Sprintf("router_fix.driver %q is not a built-in driver", rf.Driver))
		}
		if len(rf.Settings) == 0 {
			errs = append(errs, "router_fix.settings is empty")
		}
		for _, k := range rf.Settings {
			if ok && !contains(supported, k) {
				errs = append(errs, fmt.Sprintf("router_fix: driver %s cannot change %q", rf.Driver, k))
			}
		}
	}
	if secretRe.Match(data) {
		errs = append(errs, "looks like it contains a username or password; profiles must never include credentials")
	}
	if len(errs) > 0 {
		sort.Strings(errs)
		return p, fmt.Errorf("%s:\n  - %s", name, strings.Join(errs, "\n  - "))
	}
	return p, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// All returns the embedded profiles.
func All() ([]Profile, error) {
	var out []Profile
	err := fs.WalkDir(embedded, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := embedded.ReadFile(path)
		if err != nil {
			return err
		}
		p, err := Parse("profiles/"+path, b)
		if err != nil {
			return err
		}
		out = append(out, p)
		return nil
	})
	return out, err
}

// Find returns the first profile whose match rules fit the admin page title or Server header.
func Find(title, server string) *Profile {
	all, _ := All()
	title, server = strings.ToLower(title), strings.ToLower(server)
	for i, p := range all {
		for _, m := range p.Match.HTTPTitleContains {
			if title != "" && strings.Contains(title, strings.ToLower(m)) {
				return &all[i]
			}
		}
		for _, m := range p.Match.HTTPServerContains {
			if server != "" && strings.Contains(server, strings.ToLower(m)) {
				return &all[i]
			}
		}
	}
	return nil
}

// SettingFor maps a finding ID (and, for channels, its title) to the profile
// setting that fixes it, or "".
func SettingFor(findingID, title string) string { return settingFor(findingID, title) }

// settingFor maps a finding to the profile setting that fixes it.
func settingFor(findingID, title string) string {
	switch findingID {
	case "wifi-channel":
		var ch int
		fmt.Sscanf(strings.TrimPrefix(title, "Switch Wi-Fi channel to "), "%d", &ch)
		if ch > 0 && ch <= 14 {
			return "wifi_channel_2g"
		}
		return "wifi_channel_5g"
	}
	return map[string]string{
		"wifi-band": "band_steering", "dns-speed": "dns_servers", "bufferbloat": "sqm_qos",
		"telnet": "telnet", "upnp": "upnp", "http-admin": "https_admin", "wifi-security": "wifi_security",
	}[findingID]
}

// Steps returns model-specific instructions for a finding, or "".
func (p *Profile) Steps(findingID, title string) string {
	s, ok := p.Settings[settingFor(findingID, title)]
	if !ok {
		return ""
	}
	name := strings.TrimSpace(p.Vendor + " " + p.Model + " " + p.HardwareVersion)
	if s.MenuPath == "n/a" {
		return fmt.Sprintf("Your %s has no such option in its admin page.", name)
	}
	out := fmt.Sprintf("On your %s: %s.", name, s.MenuPath)
	if s.Notes != "" {
		out += " " + s.Notes
	}
	if !p.Verified {
		out += " (From the vendor manual, not yet checked on a real device.)"
	}
	return out
}
