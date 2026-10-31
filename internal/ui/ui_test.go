package ui

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kavin-bm/fixmyrouter/internal/diagnose/diagtest"
)

func TestSecurityGates(t *testing.T) {
	t.Setenv("FIXMYROUTER_HOME", t.TempDir())
	s := New("test", "127.0.0.1:7878")
	do := func(method, path, host, token, ctype, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Host = host
		if token != "" {
			r.Header.Set("X-Fixmyrouter-Token", token)
		}
		if ctype != "" {
			r.Header.Set("Content-Type", ctype)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	cases := []struct {
		name                           string
		method, path, host, tok, ctype string
		want                           int
	}{
		{"page", "GET", "/", "127.0.0.1:7878", "", "", 200},
		{"localhost page", "GET", "/", "localhost:7878", "", "", 200},
		{"dns rebinding", "GET", "/", "evil.example:7878", "", "", 403},
		{"api without token", "GET", "/api/state", "127.0.0.1:7878", "", "", 403},
		{"api wrong token", "GET", "/api/state", "127.0.0.1:7878", "nope", "", 403},
		{"api state", "GET", "/api/state", "127.0.0.1:7878", s.Token, "", 200},
		{"form post (CSRF)", "POST", "/api/undo", "127.0.0.1:7878", s.Token, "text/plain", 415},
		{"undo nothing", "POST", "/api/undo", "127.0.0.1:7878", s.Token, "application/json", 400},
		{"bad run kind", "POST", "/api/run", "127.0.0.1:7878", s.Token, "application/json", 400},
		{"static css", "GET", "/style.css", "127.0.0.1:7878", "", "", 200},
	}
	for _, c := range cases {
		body := `{"id":0}`
		if c.path == "/api/run" {
			body = `{"kind":"rm"}`
		}
		if w := do(c.method, c.path, c.host, c.tok, c.ctype, body); w.Code != c.want {
			t.Errorf("%s: got %d want %d (%s)", c.name, w.Code, c.want, w.Body.String())
		}
	}
	page := do("GET", "/", "127.0.0.1:7878", "", "", "").Body.String()
	if !strings.Contains(page, s.Token) || strings.Contains(page, "{{TOKEN}}") {
		t.Fatal("token not injected into the page")
	}
	if external(page) {
		t.Fatal("page must not reference external URLs")
	}
}

// external reports a URL other than the SVG XML namespace (an identifier, never fetched).
func external(s string) bool {
	s = strings.ReplaceAll(s, "http://www.w3.org/2000/svg", "")
	return strings.Contains(s, "http://") || strings.Contains(s, "https://")
}

func TestNoExternalURLsInAssets(t *testing.T) {
	for _, name := range []string{"static/app.js", "static/solve.js", "static/style.css", "static/index.html"} {
		b, err := static.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if external(string(b)) {
			t.Errorf("%s references an external URL", name)
		}
	}
}

func TestDiagnoseFixAndReport(t *testing.T) {
	t.Setenv("FIXMYROUTER_HOME", t.TempDir())
	f := diagtest.Healthy()
	f.W.Band, f.W.Channel, f.W.TxRateMbps = "2.4GHz", 6, 144
	zero := time.Duration(0)
	s := New("test", "127.0.0.1:7878")
	s.Prober, s.Settle = f, &zero
	s.Sleep = func(context.Context, time.Duration) error { return nil }
	post := func(path, body string) (int, map[string]any) {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Host = "127.0.0.1:7878"
		r.Header.Set("X-Fixmyrouter-Token", s.Token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		var m map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		return w.Code, m
	}
	if code, m := post("/api/fix/measure", `{"fix_id":"wifi-join-5g","confirmed":true}`); code != 400 || !strings.Contains(m["error"].(string), "diagnose first") {
		t.Fatalf("measure before diagnose: %d %v", code, m)
	}
	code, d := post("/api/diagnose", `{}`)
	if code != 200 || d["primary"] != "wifi" {
		t.Fatalf("diagnose: %d %v", code, d)
	}
	if f.Count("tput")+f.Count("bloat") != 0 {
		t.Fatal("download test without the checkbox")
	}
	if code, p := post("/api/fix/preview", `{"fix_id":"wifi-join-5g"}`); code != 200 || p["action"] == nil {
		t.Fatalf("preview: %d %v", code, p)
	}
	if code, _ := post("/api/fix/measure", `{"fix_id":"wifi-join-5g","confirmed":false}`); code != 400 {
		t.Fatal("unconfirmed fix accepted")
	}
	f.W.Band, f.W.TxRateMbps = "5GHz", 866
	code, res := post("/api/fix/measure", `{"fix_id":"wifi-join-5g","confirmed":true}`)
	if code != 200 || res["delta"].(map[string]any)["improved"] != true {
		t.Fatalf("measure: %d %v", code, res)
	}
	for _, bad := range []string{`{"samples":11,"interval_seconds":10,"no_download":true}`, `{"samples":2,"interval_seconds":5,"no_download":true}`,
		`{"samples":10,"interval_seconds":600,"no_download":true}`, `{"samples":2,"interval_seconds":10}`} {
		if code, _ := post("/api/report/isp", bad); code != 400 {
			t.Errorf("isp report %s accepted", bad)
		}
	}
	code, rep := post("/api/report/isp", `{"samples":2,"interval_seconds":10,"no_download":true}`)
	if code != 200 || !strings.Contains(rep["markdown"].(string), "# Internet connection report") || !strings.HasPrefix(rep["filename"].(string), "fixmyrouter-isp-report-") {
		t.Fatalf("isp report: %d %v", code, rep)
	}
}
