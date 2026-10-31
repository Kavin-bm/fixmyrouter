package fix

import (
	"strings"
	"testing"

	"github.com/kavinbm16/fixmyrouter/internal/checks"
)

const serviceOrder = `An asterisk (*) denotes that a network service is disabled.
(1) Thunderbolt Bridge
(Hardware Port: Thunderbolt Bridge, Device: bridge0)

(2) Wi-Fi
(Hardware Port: Wi-Fi, Device: en0)

(*) Old VPN
(Hardware Port: L2TP, Device: )
`

func TestParsers(t *testing.T) {
	if got := ServiceForDevice(serviceOrder, "en0"); got != "Wi-Fi" {
		t.Fatalf("service %q", got)
	}
	if got := ServiceForDevice(serviceOrder, "en9"); got != "" {
		t.Fatalf("unknown device gave %q", got)
	}
	if got := ParseGetDNS("There aren't any DNS Servers set on Wi-Fi.\n"); len(got) != 0 {
		t.Fatalf("automatic: %v", got)
	}
	if got := ParseGetDNS("1.1.1.1\n1.0.0.1\n"); len(got) != 2 {
		t.Fatalf("manual: %v", got)
	}
}

// fakeMac simulates networksetup's DNS state for one service.
func fakeMac(t *testing.T) *[]string {
	t.Helper()
	t.Setenv("FIXMYROUTER_HOME", t.TempDir())
	goos = "darwin"
	dns := &[]string{}
	run = func(name string, args ...string) (string, error) {
		switch args[0] {
		case "-listnetworkserviceorder":
			return serviceOrder, nil
		case "-getdnsservers":
			if len(*dns) == 0 {
				return "There aren't any DNS Servers set on Wi-Fi.", nil
			}
			return strings.Join(*dns, "\n"), nil
		case "-setdnsservers":
			if args[1] != "Wi-Fi" {
				t.Fatalf("wrong service %q", args[1])
			}
			if args[2] == "Empty" {
				*dns = nil
			} else {
				*dns = append([]string(nil), args[2:]...)
			}
			return "", nil
		}
		t.Fatalf("unexpected command %v", args)
		return "", nil
	}
	return dns
}

func TestApplyAndUndo(t *testing.T) {
	dns := fakeMac(t)
	c, err := Plan(&checks.AutoFix{ID: "set-dns", Args: []string{"1.1.1.1", "1.0.0.1"}}, "en0")
	if err != nil {
		t.Fatal(err)
	}
	if c.Service != "Wi-Fi" || len(c.Before) != 0 {
		t.Fatalf("plan %+v", c)
	}
	if len(*dns) != 0 {
		t.Fatal("Plan must not change anything")
	}
	if err := Apply(c); err != nil {
		t.Fatal(err)
	}
	if strings.Join(*dns, ",") != "1.1.1.1,1.0.0.1" {
		t.Fatalf("after apply: %v", *dns)
	}
	h, _ := History()
	if len(h) != 1 || h[0].Status != StatusApplied {
		t.Fatalf("history %+v", h)
	}
	u, err := Undo(0)
	if err != nil || u.Status != StatusUndone {
		t.Fatalf("undo %+v %v", u, err)
	}
	if len(*dns) != 0 {
		t.Fatalf("undo should restore automatic DNS, got %v", *dns)
	}
	if _, err := Undo(0); err == nil {
		t.Fatal("second undo should have nothing to do")
	}
}

func TestPlanRejects(t *testing.T) {
	fakeMac(t)
	for _, a := range []*checks.AutoFix{nil, {ID: "rm-rf"}, {ID: "set-dns"}, {ID: "set-dns", Args: []string{"1.1.1.1; reboot"}}} {
		if _, err := Plan(a, "en0"); err == nil {
			t.Errorf("accepted %+v", a)
		}
	}
	goos = "linux"
	defer func() { goos = "darwin" }()
	if _, err := Plan(&checks.AutoFix{ID: "set-dns", Args: []string{"1.1.1.1"}}, "wlan0"); err != ErrUnsupported {
		t.Fatalf("linux: %v", err)
	}
}
