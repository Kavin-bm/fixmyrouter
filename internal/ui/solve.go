package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/Kavin-bm/fixmyrouter/internal/diagnose"
	"github.com/Kavin-bm/fixmyrouter/internal/isp"
	"github.com/Kavin-bm/fixmyrouter/internal/store"
)

// Limits for the ISP report from the browser (the CLI allows longer runs).
const (
	MaxUISamples  = 10
	MaxUIDuration = 15 * time.Minute
)

var errBusy = errors.New("a measurement is already running; try again when it finishes")

func decode(r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<16)).Decode(v)
}

func (s *Server) diagnose(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Load         bool    `json:"load"`
		Throughput   bool    `json:"throughput"`
		ThroughputMB int     `json:"throughput_mb"`
		PlanMbps     float64 `json:"plan_mbps"`
	}
	if err := decode(r, &req); err != nil {
		reply(w, nil, err)
		return
	}
	if !s.busy.TryLock() {
		reply(w, nil, errBusy)
		return
	}
	defer s.busy.Unlock()
	d, err := diagnose.Run(r.Context(), s.prober(), s.Version, diagnose.Options{Load: req.Load, Throughput: req.Throughput, ThroughputMB: req.ThroughputMB, PlanMbps: req.PlanMbps})
	if err == nil {
		err = diagnose.Save(d)
	}
	reply(w, d, err)
}

type fixReq struct {
	FixID     string `json:"fix_id"`
	Confirmed bool   `json:"confirmed"`
}

// preview shows what a fix would do, without changing anything.
func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	var req fixReq
	if err := decode(r, &req); err != nil {
		reply(w, nil, err)
		return
	}
	d := diagnose.Latest()
	if d == nil {
		reply(w, nil, errors.New("run Diagnose first"))
		return
	}
	act, _ := d.FindAction(req.FixID)
	if act == nil {
		reply(w, nil, fmt.Errorf("fix %q is not in the last diagnosis", req.FixID))
		return
	}
	out := map[string]any{"action": act, "missing_test": diagnose.MissingTest(act.Metric, d.Options)}
	if act.Level == diagnose.LevelAuto {
		c, err := diagnose.PlanAuto(act)
		if err != nil {
			reply(w, nil, err)
			return
		}
		out["change"] = c
	}
	reply(w, out, nil)
}

// measure applies (auto, after the dialog's yes) or confirms (guided, after
// the person says they did it) a fix, then measures again.
func (s *Server) measure(w http.ResponseWriter, r *http.Request) {
	var req fixReq
	if err := decode(r, &req); err != nil {
		reply(w, nil, err)
		return
	}
	if !s.busy.TryLock() {
		reply(w, nil, errBusy)
		return
	}
	defer s.busy.Unlock()
	res, err := diagnose.MeasureFix(r.Context(), s.prober(), s.Version, diagnose.Latest(), req.FixID, req.Confirmed, s.settle())
	reply(w, res, err)
}

func (s *Server) ispReport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Samples         int     `json:"samples"`
		IntervalSeconds int     `json:"interval_seconds"`
		PlanMbps        float64 `json:"plan_mbps"`
		NoDownload      bool    `json:"no_download"`
		Confirmed       bool    `json:"confirmed"`
	}
	if err := decode(r, &req); err != nil {
		reply(w, nil, err)
		return
	}
	o := isp.Options{Samples: req.Samples, Interval: time.Duration(req.IntervalSeconds) * time.Second, PlanMbps: req.PlanMbps, NoDownload: req.NoDownload}
	if o.Samples > MaxUISamples {
		reply(w, nil, fmt.Errorf("at most %d samples here; use `fixmyrouter report --isp` for longer runs", MaxUISamples))
		return
	}
	if err := o.Validate(); err != nil {
		reply(w, nil, err)
		return
	}
	if d := time.Duration(o.Samples-1) * o.Interval; d > MaxUIDuration {
		reply(w, nil, fmt.Errorf("that would take %s; at most %s here, use `fixmyrouter report --isp` for longer runs", d, MaxUIDuration))
		return
	}
	if !o.NoDownload && !req.Confirmed {
		reply(w, nil, fmt.Errorf("this downloads up to %d MB; confirm first", o.DataBudgetMB()))
		return
	}
	if !s.busy.TryLock() {
		reply(w, nil, errBusy)
		return
	}
	defer s.busy.Unlock()
	rep, err := isp.Collect(r.Context(), s.prober(), s.Version, o, s.sleep(), nil)
	if err != nil {
		reply(w, nil, err)
		return
	}
	md, js, err := isp.Write(filepath.Join(store.Dir(), "reports"), rep)
	var b strings.Builder
	isp.Markdown(&b, rep)
	reply(w, map[string]any{"report": rep, "markdown": b.String(), "markdown_file": md, "json_file": js,
		"filename": strings.TrimSuffix(filepath.Base(md), ".md")}, err)
}
