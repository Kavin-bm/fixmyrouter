package profiles

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

func TestEmbeddedProfilesValid(t *testing.T) {
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) == 0 {
		t.Fatal("no profiles embedded")
	}
	p := Find("TP-Link Archer C6", "")
	if p == nil || !strings.Contains(p.Steps("upnp", ""), "NAT Forwarding") {
		t.Fatalf("archer c6 match: %+v", p)
	}
	if s := p.Steps("wifi-channel", "Switch Wi-Fi channel to 6"); !strings.Contains(s, "2.4GHz") {
		t.Fatalf("channel 6 should use the 2.4 GHz setting: %s", s)
	}
	if Find("Some Other Router", "lighttpd") != nil {
		t.Fatal("matched an unknown router")
	}
}

func TestParseRejects(t *testing.T) {
	good := "vendor: X\nmodel: Y\nfirmware_tested: '1'\ncontributed_by: me\nmatch:\n  http_title_contains: [XYZ-100]\nsettings:\n  upnp:\n    menu_path: A > B\n"
	if _, err := Parse("good", []byte(good)); err != nil {
		t.Fatalf("good profile rejected: %v", err)
	}
	for name, bad := range map[string]string{
		"unknown key":    good + "default_password: admin\n",
		"unknown set":    strings.Replace(good, "upnp:", "reboot:", 1),
		"credentials":    strings.Replace(good, "A > B", "A > B (login: admin / password: admin)", 1),
		"missing vendor": strings.Replace(good, "vendor: X\n", "", 1),
		"no match":       strings.Replace(good, "[XYZ-100]", "[]", 1),
	} {
		if _, err := Parse(name, []byte(bad)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// The JSON schema (for editors) must list the same settings as the Go validator.
func TestSchemaInSync(t *testing.T) {
	b, err := os.ReadFile("schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Properties struct {
			Settings struct {
				Properties map[string]any `json:"properties"`
			} `json:"settings"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	var got []string
	for k := range s.Properties.Settings.Properties {
		got = append(got, k)
	}
	want := append([]string(nil), Settings...)
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("schema.json settings %v != Go %v", got, want)
	}
}
