package speed

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"
)

// Bufferbloat compares internet latency when idle and while downloading.
type Bufferbloat struct {
	Target       string    `json:"target"`
	Idle         PingStats `json:"idle"`
	Loaded       PingStats `json:"loaded"`
	IncreaseMs   float64   `json:"increase_ms"`
	Grade        string    `json:"grade"`
	DownloadMbps float64   `json:"download_mbps"`
	DownloadedMB float64   `json:"downloaded_mb"`
	DownloadURL  string    `json:"download_url"`
}

// DownloadURL serves throwaway bytes for the load test (fetched repeatedly).
const DownloadURL = "https://speed.cloudflare.com/__down?bytes=25000000"

// MaxDownloadBytes caps the data the load test may use.
const MaxDownloadBytes = 200_000_000

// BloatGrade maps added latency under load to a letter grade.
func BloatGrade(increaseMs float64) string {
	switch {
	case increaseMs < 5:
		return "A+"
	case increaseMs < 30:
		return "A"
	case increaseMs < 60:
		return "B"
	case increaseMs < 200:
		return "C"
	case increaseMs < 400:
		return "D"
	}
	return "F"
}

// MeasureBufferbloat pings target idle, then again during a download of at
// most maxDur. It uses real data (up to 200 MB), so it only runs with --load.
func MeasureBufferbloat(ctx context.Context, target string, maxDur time.Duration) (Bufferbloat, error) {
	b := Bufferbloat{Target: target, DownloadURL: DownloadURL}
	idle, err := Ping(ctx, target, 10)
	if err != nil {
		return b, err
	}
	b.Idle = idle

	dctx, cancel := context.WithTimeout(ctx, maxDur)
	defer cancel()
	var n atomic.Int64
	start := time.Now()
	done := make(chan struct{})
	var end time.Time
	go func() {
		defer close(done)
		defer func() { end = time.Now() }()
		buf := make([]byte, 64<<10)
		for n.Load() < MaxDownloadBytes && dctx.Err() == nil {
			req, err := http.NewRequestWithContext(dctx, http.MethodGet, DownloadURL, nil)
			if err != nil {
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return
			}
			for {
				m, err := resp.Body.Read(buf)
				n.Add(int64(m))
				if err != nil || n.Load() >= MaxDownloadBytes {
					break
				}
			}
			resp.Body.Close()
		}
	}()
	select { // let the download ramp up
	case <-time.After(1500 * time.Millisecond):
	case <-ctx.Done():
	}
	loaded, _ := Ping(ctx, target, 20)
	cancel()
	<-done
	el := end.Sub(start).Seconds() // done is closed, so end is set
	b.Loaded = loaded
	b.DownloadedMB = round1(float64(n.Load()) / 1e6)
	if el > 0 {
		b.DownloadMbps = round1(float64(n.Load()) * 8 / 1e6 / el)
	}
	b.IncreaseMs = round1(b.Loaded.MedianMs - b.Idle.MedianMs)
	if b.IncreaseMs < 0 {
		b.IncreaseMs = 0
	}
	b.Grade = BloatGrade(b.IncreaseMs)
	return b, nil
}
