package routerfix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Kavin-bm/fixmyrouter/profiles"
)

// fakeOpenWrt is an in-memory ubus endpoint with uci get/set/delete/commit.
type fakeOpenWrt struct {
	mu        sync.Mutex
	password  string
	logins    int
	committed map[string]int
	uci       map[string]map[string]map[string]string // config -> section -> option -> value
}

func newFake() *fakeOpenWrt {
	return &fakeOpenWrt{password: "correct horse", committed: map[string]int{}, uci: map[string]map[string]map[string]string{
		"wireless": {
			"radio0":         {".type": "wifi-device", "band": "2g", "channel": "6"},
			"radio1":         {".type": "wifi-device", "band": "5g", "channel": "36"},
			"default_radio0": {".type": "wifi-iface", "device": "radio0"},
		},
		"sqm": {"eth1": {".type": "queue", "interface": "wan", "enabled": "0", "download": "85000", "upload": "10000", "qdisc": "cake"}},
	}}
}

func (f *fakeOpenWrt) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var req struct {
		Params []json.RawMessage `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	var sid, obj, method string
	_ = json.Unmarshal(req.Params[0], &sid)
	_ = json.Unmarshal(req.Params[1], &obj)
	_ = json.Unmarshal(req.Params[2], &method)
	var a map[string]any
	_ = json.Unmarshal(req.Params[3], &a)
	reply := func(status int, data any) {
		res := []any{status}
		if data != nil {
			res = append(res, data)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": res})
	}
	if obj == "session" && method == "login" {
		f.logins++
		if a["password"] != f.password {
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "error": map[string]any{"code": -32002, "message": "Access denied"}})
			return
		}
		reply(0, map[string]any{"ubus_rpc_session": "s3ss10n"})
		return
	}
	if sid != "s3ss10n" {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "error": map[string]any{"code": -32002, "message": "Access denied"}})
		return
	}
	cfg, _ := a["config"].(string)
	sec, _ := a["section"].(string)
	opt, _ := a["option"].(string)
	c, ok := f.uci[cfg]
	if !ok {
		reply(4, nil)
		return
	}
	switch method {
	case "get":
		if sec == "" {
			reply(0, map[string]any{"values": c})
			return
		}
		v, ok := c[sec][opt]
		if !ok {
			reply(4, nil)
			return
		}
		reply(0, map[string]any{"value": v})
	case "set":
		for k, v := range a["values"].(map[string]any) {
			c[sec][k] = v.(string)
		}
		reply(0, nil)
	case "delete":
		delete(c[sec], opt)
		reply(0, nil)
	case "commit":
		f.committed[cfg]++
		reply(0, nil)
	default:
		reply(3, nil)
	}
}

func setup(t *testing.T) (*fakeOpenWrt, Driver, *profiles.Profile, string) {
	t.Helper()
	t.Setenv("FIXMYROUTER_HOME", t.TempDir())
	f := newFake()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	p := profiles.Find("OpenWrt - LuCI", "")
	d, err := DriverFor(p, "wifi_channel_2g")
	if err != nil {
		t.Fatal(err)
	}
	return f, d, p, srv.URL
}

func TestChannelPlanApplyUndo(t *testing.T) {
	f, d, p, url := setup(t)
	ctx := context.Background()
	s, err := d.Login(ctx, url, "root", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	c, err := Plan(ctx, d, s, p, "192.168.1.1", "wifi_channel_2g", "11")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Edits) != 1 || c.Edits[0].Section != "radio0" || c.Edits[0].Before != "6" || c.Edits[0].After != "11" {
		t.Fatalf("plan %+v", c.Edits)
	}
	if f.uci["wireless"]["radio0"]["channel"] != "6" {
		t.Fatal("plan must not change anything")
	}
	if err := Apply(ctx, s, c); err != nil {
		t.Fatal(err)
	}
	if f.uci["wireless"]["radio0"]["channel"] != "11" || f.committed["wireless"] != 1 || f.uci["wireless"]["radio1"]["channel"] != "36" {
		t.Fatalf("apply: %v committed=%v", f.uci["wireless"], f.committed)
	}
	h, _ := History()
	if len(h) != 1 || h[0].Status != StatusApplied {
		t.Fatalf("history %+v", h)
	}
	if _, err := Undo(ctx, s, 0); err != nil {
		t.Fatal(err)
	}
	if f.uci["wireless"]["radio0"]["channel"] != "6" {
		t.Fatal("undo did not restore channel 6")
	}
	if _, err := Undo(ctx, s, 0); err == nil {
		t.Fatal("second undo should find nothing")
	}
}

func TestSQMPlanCreatesAndDeletesOnUndo(t *testing.T) {
	f, d, p, url := setup(t)
	ctx := context.Background()
	delete(f.uci["sqm"]["eth1"], "script")
	s, _ := d.Login(ctx, url, "root", "correct horse")
	c, err := Plan(ctx, d, s, p, "192.168.1.1", "sqm_qos", "90/18")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.Summary, "download 90 Mbps") {
		t.Fatal(c.Summary)
	}
	if err := Apply(ctx, s, c); err != nil {
		t.Fatal(err)
	}
	q := f.uci["sqm"]["eth1"]
	if q["enabled"] != "1" || q["download"] != "90000" || q["upload"] != "18000" || q["script"] != "piece_of_cake.qos" {
		t.Fatalf("sqm %v", q)
	}
	if _, err := Undo(ctx, s, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := q["script"]; ok || q["enabled"] != "0" || q["download"] != "85000" {
		t.Fatalf("undo sqm %v", q)
	}
}

func TestSafetyGates(t *testing.T) {
	f, d, p, url := setup(t)
	ctx := context.Background()
	if _, err := d.Login(ctx, url, "root", "admin"); err == nil || !strings.Contains(err.Error(), "does not retry") {
		t.Fatalf("wrong password: %v", err)
	}
	if f.logins != 1 {
		t.Fatalf("a wrong password must cause exactly one login attempt, got %d", f.logins)
	}
	s, _ := d.Login(ctx, url, "root", "correct horse")
	for _, bad := range [][2]string{{"wifi_channel_2g", "14"}, {"wifi_channel_5g", "6"}, {"sqm_qos", "fast"}, {"upnp", "0"}, {"wifi_channel_2g", "6"}} {
		if _, err := Plan(ctx, d, s, p, "192.168.1.1", bad[0], bad[1]); err == nil {
			t.Errorf("plan %v accepted", bad)
		}
	}
	tp := profiles.Find("Archer C6", "")
	if _, err := DriverFor(tp, "wifi_channel_2g"); err != ErrNotAllowed {
		t.Fatalf("profile without router_fix: %v", err)
	}
	if _, err := DriverFor(nil, "sqm_qos"); err != ErrNotAllowed {
		t.Fatal("no profile must mean no router change")
	}
	for _, ip := range []string{"1.1.1.1", "8.8.8.8", "example.com", ""} {
		if _, err := BaseURL(ip, false); err == nil {
			t.Errorf("BaseURL(%q) allowed a non-private address", ip)
		}
	}
	if u, err := BaseURL("192.168.1.1", false); err != nil || u != "http://192.168.1.1:80" {
		t.Fatalf("%s %v", u, err)
	}
	delete(f.uci, "sqm")
	if _, err := Plan(ctx, d, s, p, "192.168.1.1", "sqm_qos", "90"); err == nil || !strings.Contains(err.Error(), "luci-app-sqm") {
		t.Fatalf("sqm missing: %v", err)
	}
}

func TestDriversMatchProfiles(t *testing.T) {
	for name := range profiles.RouterDrivers {
		if !HasDriver(name) {
			t.Errorf("profiles allows driver %q but it is not built in", name)
		}
	}
}
