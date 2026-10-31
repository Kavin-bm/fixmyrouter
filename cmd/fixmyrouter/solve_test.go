package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Kavin-bm/fixmyrouter/internal/diagnose/diagtest"
)

func withFake(t *testing.T) *diagtest.Fake {
	t.Helper()
	t.Setenv("FIXMYROUTER_HOME", t.TempDir())
	f := diagtest.Healthy()
	old, oldSleep := prober, sleep
	prober = f
	sleep = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() { prober, sleep = old, oldSleep })
	return f
}

func TestFixMeasureDeclineChangesNothing(t *testing.T) {
	f := withFake(t)
	f.DNSMs["192.168.1.1"] = 150
	// "no" at the prompt: the automatic DNS fix is planned but not applied.
	// Planning may fail on CI machines without macOS networksetup; either
	// way, nothing may be applied.
	_ = runFixMeasure(context.Background(), []string{"--measure", "set-dns"}, strings.NewReader("no\n"))
	if _, err := os.Stat(filepath.Join(os.Getenv("FIXMYROUTER_HOME"), "changes.json")); err == nil {
		b, _ := os.ReadFile(filepath.Join(os.Getenv("FIXMYROUTER_HOME"), "changes.json"))
		if strings.Contains(string(b), "applied") {
			t.Fatal("a declined fix was applied")
		}
	}
}

func TestFixMeasureGuidedThenRemeasure(t *testing.T) {
	f := withFake(t)
	f.W.Band, f.W.Channel = "2.4GHz", 6
	// Guided/advice fix: wait for Enter, measure again, compare.
	if err := runFixMeasure(context.Background(), []string{"--measure", "wifi-join-5g"}, strings.NewReader("\n")); err != nil {
		t.Fatal(err)
	}
	if err := runFixMeasure(context.Background(), []string{"--measure", "nope"}, strings.NewReader("\n")); err == nil {
		t.Fatal("unknown fix id accepted")
	}
}

func TestFixMeasureNeedsConsentForDownloads(t *testing.T) {
	f := withFake(t)
	f.Bloat = nil
	f.Pings["1.1.1.1"], f.Pings["8.8.8.8"] = diagtest.Ping(150, 40, 5), diagtest.Ping(160, 40, 5)
	err := runFixMeasure(context.Background(), []string{"--measure", "isp-report"}, strings.NewReader("\n"))
	if err != nil {
		t.Fatal(err) // no metric: prints the command, measures nothing more
	}
	if f.Count("bloat")+f.Count("tput") != 0 {
		t.Fatal("download test ran without a flag")
	}
}

func TestReportISP(t *testing.T) {
	withFake(t)
	dir := t.TempDir()
	if err := runReport(context.Background(), []string{"--isp", "--samples", "2", "--interval", "10s", "--out", dir}, strings.NewReader("no\n")); err != nil {
		t.Fatal(err)
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "*")); len(m) != 0 {
		t.Fatal("declined report still ran")
	}
	if err := runReport(context.Background(), []string{"--isp", "--samples", "2", "--interval", "10s", "--out", dir, "--yes", "--no-download"}, strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "fixmyrouter-isp-report-*")); len(m) != 2 {
		t.Fatalf("files: %v", m)
	}
}
