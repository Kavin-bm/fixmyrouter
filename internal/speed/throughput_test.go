package speed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMeasureThroughputCap(t *testing.T) {
	var requested atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.ParseInt(r.URL.Query().Get("bytes"), 10, 64)
		requested.Store(n)
		chunk := []byte(strings.Repeat("x", 32<<10))
		for sent := int64(0); sent < n; sent += int64(len(chunk)) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()
	tp, err := MeasureThroughput(context.Background(), srv.Client(), srv.URL+"/?bytes=%d", 2, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if requested.Load() != 2_000_000/ThroughputStreams {
		t.Fatalf("per-request bytes %d", requested.Load())
	}
	if !tp.HitCap || tp.DownloadedMB < 2 || tp.DownloadedMB > 2.6 || tp.DownloadMbps <= 0 || tp.CapMB != 2 {
		t.Fatalf("%+v", tp)
	}
}

func TestMeasureThroughputErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	if _, err := MeasureThroughput(context.Background(), srv.Client(), srv.URL+"/?bytes=%d", 1, 2*time.Second); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("want HTTP 403 error, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := MeasureThroughput(ctx, srv.Client(), srv.URL+"/?bytes=%d", 1, 2*time.Second); err == nil {
		t.Fatal("cancelled run must fail")
	}
}

func TestMeasureThroughputClampsCap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tp, _ := MeasureThroughput(ctx, nil, "http://127.0.0.1:1/?bytes=%d", 10_000, time.Second)
	if tp.CapMB != MaxThroughputMB {
		t.Fatalf("cap %v", tp.CapMB)
	}
}
