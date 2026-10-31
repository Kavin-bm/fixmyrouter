package isp

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Kavin-bm/fixmyrouter/internal/diagnose/diagtest"
)

func TestReportBlamesISPOnlyWithHealthyLAN(t *testing.T) {
	f := diagtest.Healthy()
	f.Pings["1.1.1.1"], f.Pings["8.8.8.8"] = diagtest.Ping(140, 30, 5), diagtest.Ping(150, 30, 6)
	f.TputMbps = 20
	var slept []time.Duration
	sleep := func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }
	n := 0
	r, err := Collect(context.Background(), f, "test", Options{Samples: 3, Interval: time.Minute, PlanMbps: 100}, sleep, func(int, Sample) { n++ })
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Samples) != 3 || n != 3 || len(slept) != 2 || slept[0] != time.Minute || f.Count("tput") != 3 {
		t.Fatalf("samples=%d progress=%d slept=%v tput=%d", len(r.Samples), n, slept, f.Count("tput"))
	}
	if r.Summary.BelowPlan != 3 || r.Summary.LocalOKSamples != 3 || !strings.Contains(r.Conclusion, "points to the connection beyond the router") {
		t.Fatalf("%+v %s", r.Summary, r.Conclusion)
	}
	dir := t.TempDir()
	md, js, err := Write(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(md)
	for _, want := range []string{"# Internet connection report", "Plan download speed: **100 Mbps**", "| 20.0 / 20.0 / 20.0 Mbps |", "20% of the 100 Mbps plan"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("markdown missing %q:\n%s", want, b)
		}
	}
	var back Report
	if b, _ := os.ReadFile(js); json.Unmarshal(b, &back) != nil || len(back.Samples) != 3 {
		t.Fatal("json")
	}
}

func TestLocalProblemsAreDisclosed(t *testing.T) {
	f := diagtest.Healthy()
	f.Pings["192.168.1.1"] = diagtest.Ping(40, 30, 10)
	f.Pings["1.1.1.1"], f.Pings["8.8.8.8"] = diagtest.Ping(200, 30, 5), diagtest.Ping(210, 30, 5)
	r, err := Collect(context.Background(), f, "test", Options{Samples: 1, NoDownload: true}, RealSleep, nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.Count("tput") != 0 || !strings.Contains(r.Conclusion, "may be local") {
		t.Fatalf("tput=%d %s", f.Count("tput"), r.Conclusion)
	}
}

func TestLimitsAndCancel(t *testing.T) {
	for _, o := range []Options{{Samples: 0}, {Samples: MaxSamples + 1}, {Samples: 2, Interval: time.Second}} {
		if err := o.Validate(); err == nil {
			t.Errorf("%+v accepted", o)
		}
	}
	o := Options{Samples: 4, Interval: time.Minute, ThroughputMB: 999}
	if o.Validate() != nil || o.ThroughputMB != 200 || o.DataBudgetMB() != 800 {
		t.Fatalf("%+v", o)
	}
	ctx, cancel := context.WithCancel(context.Background())
	sleep := func(context.Context, time.Duration) error { cancel(); return context.Canceled }
	f := diagtest.Healthy()
	f.TputErr = diagtest.ErrFake
	r, err := Collect(ctx, f, "test", Options{Samples: 5, Interval: time.Minute}, sleep, nil)
	if err != nil || len(r.Samples) != 1 || !strings.Contains(strings.Join(r.Samples[0].Problems, ""), "download test failed") {
		t.Fatalf("partial report after cancel: %v %+v", err, r)
	}
}
