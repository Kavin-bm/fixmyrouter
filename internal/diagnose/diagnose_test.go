package diagnose

import (
	"context"
	"strings"
	"testing"

	"github.com/Kavin-bm/fixmyrouter/internal/discover"
	"github.com/Kavin-bm/fixmyrouter/internal/speed"
	"github.com/Kavin-bm/fixmyrouter/profiles"
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
	Calls    map[string]int
}

func (f *Fake) count(k string) {
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
	return speed.Throughput{DownloadMbps: f.TputMbps, CapMB: float64(capMB)}, nil
}
func (f *Fake) GOOS() string { return f.OS }

func ping(med, jit, loss float64) speed.PingStats {
	return speed.PingStats{Received: 10, MedianMs: med, JitterMs: jit, LossPct: loss}
}

// healthy is a good home network: 5 GHz, strong signal, fast router, fast DNS.
func healthy() *Fake {
	return &Fake{
		GW: discover.Gateway{IP: "192.168.1.1"},
		W:  &speed.WiFi{Band: "5GHz", Channel: 36, SignalDBm: -50, TxRateMbps: 866, Supports5GHz: true},
		Pings: map[string]speed.PingStats{
			"192.168.1.1": ping(2, 1, 0), "1.1.1.1": ping(12, 2, 0), "8.8.8.8": ping(15, 2, 0),
		},
		Sys:   []string{"192.168.1.1"},
		DNSMs: map[string]float64{"192.168.1.1": 10, "1.1.1.1": 9, "8.8.8.8": 14, "9.9.9.9": 20},
		OS:    "darwin",
	}
}

func run(t *testing.T, f *Fake, opt Options) *Diagnosis {
	t.Helper()
	d, err := Run(context.Background(), f, "test", opt)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestHealthy(t *testing.T) {
	f := healthy()
	d := run(t, f, Options{})
	if !d.Healthy || d.Primary != "" || !strings.Contains(d.Verdict, "No bottleneck") {
		t.Fatalf("%+v", d)
	}
	if f.Calls["bloat"]+f.Calls["tput"] != 0 {
		t.Fatal("download tests must not run without consent")
	}
	if !strings.Contains(strings.Join(d.Notes, " "), "--throughput") {
		t.Fatalf("notes should say what was not tested: %v", d.Notes)
	}
}

func TestWiFiPrimary(t *testing.T) {
	f := healthy()
	f.W = &speed.WiFi{Band: "2.4GHz", Channel: 6, SignalDBm: -79, TxRateMbps: 58, Supports5GHz: true,
		NearbyChannels: []speed.Nearby{{Channel: 6, Band: "2.4GHz"}, {Channel: 6, Band: "2.4GHz"}, {Channel: 7, Band: "2.4GHz"}, {Channel: 5, Band: "2.4GHz"}, {Channel: 1, Band: "2.4GHz"}}}
	f.Pings["192.168.1.1"] = ping(25, 18, 4)
	d := run(t, f, Options{})
	p := d.PrimaryBottleneck()
	if d.Primary != KindWiFi || p.Impact < 70 {
		t.Fatalf("primary %q %+v", d.Primary, p)
	}
	for _, want := range []string{"wifi-signal", "wifi-band", "wifi-channel", "wifi-linkrate", "link-unstable"} {
		if !strings.Contains(strings.Join(p.Issues, ","), want) {
			t.Errorf("missing issue %s in %v", want, p.Issues)
		}
	}
	for _, b := range d.Bottlenecks {
		if b.Kind == KindRouter {
			t.Fatal("unstable router ping on bad Wi-Fi must count as Wi-Fi, not router")
		}
	}
	var ids []string
	for _, a := range p.Actions {
		ids = append(ids, a.Level+":"+a.ID)
	}
	got := strings.Join(ids, " ")
	if !strings.Contains(got, "advice:channel-advice") || !strings.Contains(got, "advice:wifi-placement") || strings.Contains(got, "router:") {
		t.Fatalf("actions without a profile: %s", got)
	}
}

func TestChannelFixLevelsFollowProfile(t *testing.T) {
	m := Measurements{Local: &speed.Report{Congestion: &speed.Congestion{Band: "2.4GHz", OnMyChannel: 5, BestChannel: 11, BestCount: 1}}}
	b := &Bottleneck{Issues: []string{"wifi-channel"}}
	levels := func(p *profiles.Profile) string {
		var s []string
		for _, a := range Actions(b, m, p, "darwin") {
			s = append(s, a.Level)
		}
		return strings.Join(s, ",")
	}
	if got := levels(nil); got != "advice" {
		t.Fatalf("no profile: %s", got)
	}
	if got := levels(profiles.Find("Archer C6", "")); got != "guided,advice" {
		t.Fatalf("TP-Link (guided only): %s", got)
	}
	ow := profiles.Find("OpenWrt", "")
	acts := Actions(b, m, ow, "darwin")
	if acts[0].Level != LevelRouter || acts[0].RouterSetting != "wifi_channel_2g" || acts[0].RouterValue != "11" {
		t.Fatalf("OpenWrt: %+v", acts[0])
	}
	if !strings.Contains(strings.Join(acts[1].Steps, " "), "Choose channel 11") {
		t.Fatalf("guided steps: %v", acts[1].Steps)
	}
}

func TestBufferbloatAndSQMRate(t *testing.T) {
	f := healthy()
	f.Bloat = &speed.Bufferbloat{Idle: ping(12, 1, 0), Loaded: ping(260, 40, 0), IncreaseMs: 248, Grade: "D", DownloadMbps: 100}
	d := run(t, f, Options{Load: true})
	if d.Primary != KindBufferbloat {
		t.Fatalf("primary %q", d.Primary)
	}
	a := d.PrimaryBottleneck().Actions
	if a[len(a)-1].ID != "sqm-advice" || !strings.Contains(strings.Join(a[len(a)-1].Steps, " "), "about 90 Mbps") {
		t.Fatalf("sqm advice should give a concrete rate: %+v", a)
	}
}

func TestDNSAutoFixPicksFastestReliable(t *testing.T) {
	f := healthy()
	f.DNSMs = map[string]float64{"192.168.1.1": 120, "1.1.1.1": 30, "8.8.8.8": 18} // 9.9.9.9 does not answer
	d := run(t, f, Options{})
	if d.Primary != KindDNS {
		t.Fatalf("primary %q", d.Primary)
	}
	act, _ := d.FindAction("set-dns")
	if act == nil || act.Level != LevelAuto || act.AutoFix.Args[0] != "8.8.8.8" || act.AutoFix.Args[1] != "8.8.4.4" {
		t.Fatalf("set-dns %+v", act)
	}
	f.OS = "linux"
	d = run(t, f, Options{})
	if a, _ := d.FindAction("set-dns"); a != nil {
		t.Fatal("automatic DNS change is macOS-only")
	}
}

func TestISPOnlyWhenLANIsGood(t *testing.T) {
	f := healthy()
	f.Pings["1.1.1.1"], f.Pings["8.8.8.8"] = ping(140, 45, 6), ping(150, 50, 8)
	f.TputMbps = 30
	d := run(t, f, Options{Throughput: true, PlanMbps: 100})
	p := d.PrimaryBottleneck()
	if d.Primary != KindISP || p.Impact != 70 || p.Actions[0].ID != "isp-report" {
		t.Fatalf("primary %q %+v", d.Primary, p)
	}
	if !strings.Contains(p.Headline, "under half of your 100 Mbps plan") {
		t.Fatal(p.Headline)
	}
	// Same internet numbers, but the local link is bad: don't blame the ISP first.
	f.Pings["192.168.1.1"] = ping(30, 25, 5)
	d = run(t, f, Options{Throughput: true, PlanMbps: 100})
	if d.Primary == KindISP {
		t.Fatalf("ISP blamed while the LAN is unstable: %+v", d.Bottlenecks)
	}
	// No internet at all.
	f = healthy()
	delete(f.Pings, "1.1.1.1")
	delete(f.Pings, "8.8.8.8")
	if d := run(t, f, Options{}); d.Primary != KindISP || !strings.Contains(d.Verdict, "No reply from the internet") {
		t.Fatalf("wan down: %s", d.Verdict)
	}
}

func TestRouterBottleneckOnCable(t *testing.T) {
	f := healthy()
	f.W = nil
	f.Pings["192.168.1.1"] = ping(35, 20, 5)
	d := run(t, f, Options{})
	if d.Primary != KindRouter || d.PrimaryBottleneck().Name != "Router" {
		t.Fatalf("%q %+v", d.Primary, d.Bottlenecks)
	}
}

func TestHotspotNote(t *testing.T) {
	f := healthy()
	f.GW.IP = "172.20.10.1"
	f.Pings["172.20.10.1"] = f.Pings["192.168.1.1"]
	d := run(t, f, Options{})
	if !strings.Contains(d.Notes[0], "phone hotspot") {
		t.Fatalf("notes %v", d.Notes)
	}
}

func TestCompareIsHonest(t *testing.T) {
	before := run(t, healthy(), Options{})
	before.Measurements.Local.DNS[0].MedianMs = 48
	after := run(t, healthy(), Options{})
	after.Measurements.Local.DNS[0].MedianMs = 12
	dl := Compare(before, after, MetricDNS)
	if !dl.Improved || !strings.Contains(dl.Verdict, "DNS lookup 48 ms -> 12 ms (-75%)") {
		t.Fatalf("%+v", dl)
	}
	// Noise: 48 -> 46 ms is not an improvement.
	after.Measurements.Local.DNS[0].MedianMs = 46
	if dl := Compare(before, after, MetricDNS); dl.Improved || !strings.Contains(dl.Verdict, "No measurable gain") {
		t.Fatalf("noise counted as gain: %+v", dl)
	}
	// Worse.
	after.Measurements.Local.DNS[0].MedianMs = 90
	if dl := Compare(before, after, MetricDNS); dl.Improved || !strings.Contains(dl.Verdict, "got worse") {
		t.Fatalf("%+v", dl)
	}
	// Target not measured both times: no claim.
	if dl := Compare(before, after, MetricLoaded); dl.Improved || !strings.Contains(dl.Verdict, "Can't tell") {
		t.Fatalf("%+v", dl)
	}
}
