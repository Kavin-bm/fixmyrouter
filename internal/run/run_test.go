package run

import (
	"testing"
	"time"

	"github.com/kavinbm16/fixmyrouter/internal/checks"
	"github.com/kavinbm16/fixmyrouter/internal/report"
)

func TestSortFindings(t *testing.T) {
	fs := []checks.Finding{
		{ID: "a", Status: checks.StatusPass, Severity: "info"},
		{ID: "b", Status: checks.StatusWarn, Severity: "medium"},
		{ID: "c", Status: checks.StatusError, Severity: "info"},
		{ID: "d", Status: checks.StatusWarn, Severity: "high"},
		{ID: "e", Status: checks.StatusFail, Severity: "high"},
	}
	sortFindings(fs)
	got := ""
	for _, f := range fs {
		got += f.ID
	}
	if got != "edbca" {
		t.Fatalf("order %s", got)
	}
}

func TestHistoryAndPrevious(t *testing.T) {
	t.Setenv("FIXMYROUTER_HOME", t.TempDir())
	t0 := time.Now().UTC()
	fix := &checks.AutoFix{ID: "set-dns", Args: []string{"1.1.1.1"}}
	for i, kind := range []string{KindSpeed, KindAudit, KindSpeed} {
		a := &report.Audit{Kind: kind, Score: 50 + i, GeneratedAt: t0.Add(time.Duration(i) * time.Minute)}
		if i == 2 {
			a.Findings = []checks.Finding{{ID: "dns-speed", AutoFix: fix}}
		}
		if err := Save(a); err != nil {
			t.Fatal(err)
		}
	}
	runs, _ := History()
	if len(runs) != 3 {
		t.Fatalf("%d runs", len(runs))
	}
	if p := Previous(&runs[2]); p == nil || p.Score != 50 {
		t.Fatalf("previous speed run: %+v", p)
	}
	if f, _ := FindAutoFix("dns-speed"); f == nil || f.AutoFix.Args[0] != "1.1.1.1" {
		t.Fatalf("autofix %+v", f)
	}
	for i := 0; i < keepRuns+5; i++ {
		_ = Save(&report.Audit{Kind: KindAudit})
	}
	if runs, _ := History(); len(runs) != keepRuns {
		t.Fatalf("kept %d", len(runs))
	}
}
