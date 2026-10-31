package ui

import (
	"net/http/httptest"
	"strings"
	"testing"
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
	for _, name := range []string{"static/app.js", "static/style.css", "static/index.html"} {
		b, err := static.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if external(string(b)) {
			t.Errorf("%s references an external URL", name)
		}
	}
}
