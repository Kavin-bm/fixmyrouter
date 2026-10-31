// Package diagtest has a scripted network Prober for tests: no network, no OS commands.
package diagtest

import (
	"context"
	"errors"
	"sync"

	"github.com/Kavin-bm/fixmyrouter/internal/discover"
	"github.com/Kavin-bm/fixmyrouter/internal/speed"
)

// Fake is a scripted Prober: no network, no OS commands.
type Fake struct {
	GW       discover.Gateway
	W        *speed.WiFi
	Pings    map[string]speed.PingStats
	Sys      []string
	DNSMs    map[string]float64 // server -> median; missing = no answer
	Bloat    *speed.Bufferbloat
	TputMbps float64
	OS       string
	TputErr  error
	Calls    map[string]int
	mu       sync.Mutex
}

func (f *Fake) count(k string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Calls == nil {
		f.Calls = map[string]int{}
	}
	f.Calls[k]++
}
func (f *Fake) Gateway(context.Context) (*discover.Gateway, error) { g := f.GW; return &g, nil }
func (f *Fake) WiFi() (speed.WiFi, bool) {
	if f.W == nil {
		return speed.WiFi{}, false
	}
	return *f.W, true
}
func (f *Fake) Ping(_ context.Context, t string, n int) (speed.PingStats, error) {
	if p, ok := f.Pings[t]; ok {
		p.Target, p.Sent = t, n
		return p, nil
	}
	return speed.PingStats{Target: t, Sent: n, LossPct: 100}, nil
}
func (f *Fake) SystemResolvers() []string { return f.Sys }
func (f *Fake) DNS(_ context.Context, name, server string, rounds int) speed.DNSResult {
	q := rounds * 5
	if ms, ok := f.DNSMs[server]; ok {
		return speed.DNSResult{Name: name, Server: server, MedianMs: ms, Queries: q}
	}
	return speed.DNSResult{Name: name, Server: server, Failures: 3, Queries: 3}
}
func (f *Fake) Bufferbloat(context.Context) (speed.Bufferbloat, error) {
	f.count("bloat")
	return *f.Bloat, nil
}
func (f *Fake) Throughput(_ context.Context, capMB int) (speed.Throughput, error) {
	f.count("tput")
	if f.TputErr != nil {
		return speed.Throughput{}, f.TputErr
	}
	return speed.Throughput{DownloadMbps: f.TputMbps, CapMB: float64(capMB)}, nil
}
func (f *Fake) GOOS() string { return f.OS }

// Count returns how often a download test ran ("bloat" or "tput").
func (f *Fake) Count(k string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Calls[k]
}

// ErrFake is a canned error.
var ErrFake = errors.New("fake failure")

func Ping(med, jit, loss float64) speed.PingStats {
	return speed.PingStats{Received: 10, MedianMs: med, JitterMs: jit, LossPct: loss}
}

// Healthy is a good home network: 5 GHz, strong signal, fast router, fast DNS.
func Healthy() *Fake {
	return &Fake{
		GW: discover.Gateway{IP: "192.168.1.1"},
		W:  &speed.WiFi{Band: "5GHz", Channel: 36, SignalDBm: -50, TxRateMbps: 866, Supports5GHz: true},
		Pings: map[string]speed.PingStats{
			"192.168.1.1": Ping(2, 1, 0), "1.1.1.1": Ping(12, 2, 0), "8.8.8.8": Ping(15, 2, 0),
		},
		Sys:   []string{"192.168.1.1"},
		DNSMs: map[string]float64{"192.168.1.1": 10, "1.1.1.1": 9, "8.8.8.8": 14, "9.9.9.9": 20},
		OS:    "darwin",
	}
}
