// Package ui serves the local web UI on 127.0.0.1. All files are embedded in
// the binary; the page loads nothing from the internet.
//
// Any website you visit can try to send requests to 127.0.0.1, so every API
// call must carry a per-process token that only the page we serve knows, and
// the Host header must be our own address (blocks DNS rebinding).
package ui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Kavin-bm/fixmyrouter/internal/checks"
	"github.com/Kavin-bm/fixmyrouter/internal/diagnose"
	"github.com/Kavin-bm/fixmyrouter/internal/discover"
	"github.com/Kavin-bm/fixmyrouter/internal/fix"
	"github.com/Kavin-bm/fixmyrouter/internal/isp"
	"github.com/Kavin-bm/fixmyrouter/internal/report"
	"github.com/Kavin-bm/fixmyrouter/internal/run"
)

//go:embed static
var static embed.FS

// Server is the UI's HTTP handler.
type Server struct {
	Version string
	Token   string
	Addr    string // host:port we listen on, for the Host check
	busy    sync.Mutex

	// Prober, Sleep and Settle are swapped in tests (nil/zero = real network).
	Prober diagnose.Prober
	Sleep  isp.Sleep
	Settle *time.Duration
}

func (s *Server) prober() diagnose.Prober {
	if s.Prober != nil {
		return s.Prober
	}
	return diagnose.Real{}
}

func (s *Server) sleep() isp.Sleep {
	if s.Sleep != nil {
		return s.Sleep
	}
	return isp.RealSleep
}

func (s *Server) settle() time.Duration {
	if s.Settle != nil {
		return *s.Settle
	}
	return 3 * time.Second
}

// New returns a server with a fresh random token.
func New(version, addr string) *Server {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return &Server{Version: version, Token: hex.EncodeToString(b), Addr: addr}
}

// ListenAndServe binds to 127.0.0.1 only and serves until ctx is done.
func ListenAndServe(ctx context.Context, version string, port int, ready func(url string)) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil && port != 0 {
		ln, err = net.Listen("tcp", "127.0.0.1:0") // port taken: pick a free one
	}
	if err != nil {
		return err
	}
	s := New(version, ln.Addr().String())
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second}
	go func() { <-ctx.Done(); srv.Close() }()
	ready("http://" + s.Addr + "/")
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	_, port, _ := net.SplitHostPort(s.Addr)
	if host != s.Addr && host != "localhost:"+port {
		http.Error(w, "forbidden host", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		s.static(w, r)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Fixmyrouter-Token")), []byte(s.Token)) != 1 {
		http.Error(w, "missing or wrong token", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodPost && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "json only", http.StatusUnsupportedMediaType)
		return
	}
	switch r.Method + " " + r.URL.Path {
	case "GET /api/state":
		s.state(w)
	case "POST /api/run":
		s.run(w, r)
	case "POST /api/fix/plan":
		s.fix(w, r, false)
	case "POST /api/fix/apply":
		s.fix(w, r, true)
	case "POST /api/diagnose":
		s.diagnose(w, r)
	case "POST /api/fix/preview":
		s.preview(w, r)
	case "POST /api/fix/measure":
		s.measure(w, r)
	case "POST /api/report/isp":
		s.ispReport(w, r)
	case "POST /api/undo":
		s.undo(w, r)
	default:
		http.NotFound(w, r)
	}
}

// static serves embedded files; index.html gets the token in a meta tag.
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	sub, _ := fs.Sub(static, "static")
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.FileServer(http.FS(sub)).ServeHTTP(w, r)
		return
	}
	b, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(strings.Replace(string(b), "{{TOKEN}}", s.Token, 1)))
}

func reply(w http.ResponseWriter, v any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		v = map[string]string{"error": err.Error()}
	}
	_ = json.NewEncoder(w).Encode(v)
}

type state struct {
	Version  string        `json:"version"`
	Latest   *report.Audit `json:"latest"`
	Previous *report.Audit `json:"previous"`
	Scores   []score       `json:"scores"`
	Changes  []fix.Change  `json:"changes"`
	// Diagnosis is the latest `diagnose` result, if any.
	Diagnosis *diagnose.Diagnosis `json:"diagnosis"`
}

type score struct {
	Kind  string    `json:"kind"`
	At    time.Time `json:"at"`
	Score int       `json:"score"`
}

func (s *Server) state(w http.ResponseWriter) {
	runs, err := run.History()
	if err != nil {
		reply(w, nil, err)
		return
	}
	st := state{Version: s.Version, Scores: []score{}}
	for _, r := range runs {
		st.Scores = append(st.Scores, score{r.Kind, r.GeneratedAt, r.Score})
	}
	if n := len(runs); n > 0 {
		st.Latest = &runs[n-1]
		st.Previous = run.Previous(st.Latest)
	}
	st.Diagnosis = diagnose.Latest()
	st.Changes, err = fix.History()
	if st.Changes == nil {
		st.Changes = []fix.Change{}
	}
	reply(w, st, err)
}

func (s *Server) run(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind string `json:"kind"`
		Load bool   `json:"load"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		reply(w, nil, err)
		return
	}
	if req.Kind != run.KindCheck && req.Kind != run.KindSpeed && req.Kind != run.KindAudit {
		reply(w, nil, fmt.Errorf("kind must be check, speed or audit"))
		return
	}
	if !s.busy.TryLock() {
		w.WriteHeader(http.StatusConflict)
		reply(w, map[string]string{"error": "a check is already running"}, nil)
		return
	}
	defer s.busy.Unlock()
	a, err := run.Run(r.Context(), s.Version, run.Options{Kind: req.Kind, Load: req.Load && req.Kind != run.KindAudit})
	if err != nil {
		reply(w, nil, err)
		return
	}
	prev := run.Previous(a)
	err = run.Save(a)
	reply(w, map[string]any{"latest": a, "previous": prev}, err)
}

// fix plans (read-only) or applies the auto-fix of a saved finding. The page
// only calls apply after the person confirmed the plan in a dialog.
func (s *Server) fix(w http.ResponseWriter, r *http.Request, apply bool) {
	var req struct {
		FindingID string `json:"finding_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		reply(w, nil, err)
		return
	}
	f, _ := run.FindAutoFix(req.FindingID)
	if f == nil {
		reply(w, nil, fmt.Errorf("no automatic fix for %q in your saved results", req.FindingID))
		return
	}
	c, err := plan(f.AutoFix)
	if err != nil || !apply {
		reply(w, c, err)
		return
	}
	if !s.busy.TryLock() {
		reply(w, nil, fmt.Errorf("a check is running; try again when it finishes"))
		return
	}
	defer s.busy.Unlock()
	err = fix.Apply(c)
	reply(w, c, err)
}

func plan(a *checks.AutoFix) (*fix.Change, error) {
	_, iface, err := discover.DefaultGateway()
	if err != nil {
		return nil, err
	}
	return fix.Plan(a, iface)
}

func (s *Server) undo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		reply(w, nil, err)
		return
	}
	c, err := fix.Undo(req.ID)
	reply(w, c, err)
}
