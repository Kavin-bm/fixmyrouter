package speed

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Throughput is a WAN download measurement against a public CDN.
type Throughput struct {
	URL          string  `json:"url"`
	Streams      int     `json:"streams"`
	DownloadMbps float64 `json:"download_mbps"`
	DownloadedMB float64 `json:"downloaded_mb"`
	Seconds      float64 `json:"seconds"`
	CapMB        float64 `json:"cap_mb"`
	HitCap       bool    `json:"hit_cap"` // stopped at the data cap: the line may be faster than measured
}

// ThroughputURL is Cloudflare's speed-test endpoint; %d is the bytes per request.
const ThroughputURL = "https://speed.cloudflare.com/__down?bytes=%d"

// DefaultThroughputMB is the default data cap for one throughput test.
const DefaultThroughputMB = 25

// MaxThroughputMB is the most one throughput test may ever download.
const MaxThroughputMB = 200

// ThroughputStreams is how many parallel downloads run (one stream often
// cannot fill a fast line).
const ThroughputStreams = 4

// MeasureThroughput downloads from urlFmt (a %d placeholder for the bytes per
// request) on ThroughputStreams parallel connections until capMB megabytes or
// maxDur, whichever comes first. The speed is bytes divided by the time from
// the first byte to the end, so connection setup does not count.
func MeasureThroughput(ctx context.Context, client *http.Client, urlFmt string, capMB int, maxDur time.Duration) (Throughput, error) {
	if capMB <= 0 {
		capMB = DefaultThroughputMB
	}
	if capMB > MaxThroughputMB {
		capMB = MaxThroughputMB
	}
	if client == nil {
		client = http.DefaultClient
	}
	capBytes := int64(capMB) * 1_000_000
	perReq := capBytes / ThroughputStreams
	url := fmt.Sprintf(urlFmt, perReq)
	t := Throughput{URL: url, Streams: ThroughputStreams, CapMB: float64(capMB)}

	dctx, cancel := context.WithTimeout(ctx, maxDur)
	defer cancel()
	var n atomic.Int64
	var first atomic.Int64 // unix nanos of the first byte
	var wg sync.WaitGroup
	errs := make(chan error, ThroughputStreams)
	for i := 0; i < ThroughputStreams; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, 64<<10)
			for n.Load() < capBytes && dctx.Err() == nil {
				req, err := http.NewRequestWithContext(dctx, http.MethodGet, url, nil)
				if err != nil {
					errs <- err
					return
				}
				resp, err := client.Do(req)
				if err != nil {
					if dctx.Err() == nil {
						errs <- err
					}
					return
				}
				if resp.StatusCode != http.StatusOK {
					resp.Body.Close()
					errs <- fmt.Errorf("download test: HTTP %d", resp.StatusCode)
					return
				}
				for {
					m, err := resp.Body.Read(buf)
					if m > 0 {
						first.CompareAndSwap(0, time.Now().UnixNano())
						if n.Add(int64(m)) >= capBytes {
							cancel() // cap reached: stop every stream
						}
					}
					if err != nil {
						break
					}
				}
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()
	end := time.Now()
	close(errs)
	got := n.Load()
	if got == 0 {
		if err := <-errs; err != nil {
			return t, err
		}
		if err := ctx.Err(); err != nil {
			return t, err
		}
		return t, fmt.Errorf("download test received no data")
	}
	if err := ctx.Err(); err != nil {
		return t, err // cancelled by the person, not finished
	}
	el := end.Sub(time.Unix(0, first.Load())).Seconds()
	t.DownloadedMB = round1(float64(got) / 1e6)
	t.Seconds = round1(el)
	t.HitCap = got >= capBytes
	if el > 0 {
		t.DownloadMbps = round1(float64(got) * 8 / 1e6 / el)
	}
	return t, nil
}
