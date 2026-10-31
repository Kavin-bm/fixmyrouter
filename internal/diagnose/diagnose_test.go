package diagnose

import (
	"context"
	"strings"
	"testing"

	"github.com/Kavin-bm/fixmyrouter/internal/diagnose/diagtest"
	"github.com/Kavin-bm/fixmyrouter/internal/speed"
	"github.com/Kavin-bm/fixmyrouter/profiles"
)

func run(t *testing.T, f *diagtest.Fake, opt Options) *Diagnosis {
	t.Helper()
	d, err := Run(context.Background(), f, "test", opt)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestHealthy(t *testing.T) {
	f := diagtest.Healthy()
	d := run(t, f, Options{})
	if !d.Healthy || d.Primary != "" || !strings.Contains(d.Verdict, "No bottleneck") {
		t.Fatalf("%+v", d)
	}
	if f.Count("bloat")+f.Count("tput") != 0 {
		t.Fatal("download tests must not run without consent")
	}
	if !strings.Contains(strings.Join(d.Notes, " "), "--throughput") {
		t.Fatalf("notes should say what was not tested: %v", d.Notes)
	}
}

func TestWiFiPrimary(t *testing.T) {
	f := diagtest.Healthy()
	f.W = &speed.WiFi{Band: "2.4GHz", Channel: 6, SignalDBm: -79, TxRateMbps: 58, Supports5GHz: true,
		NearbyChannels: []speed.Nearby{{Channel: 6, Band: "2.4GHz"}, {Channel: 6, Band: "2.4GHz"}, {Channel: 7, Band: "2.4GHz"}, {Channel: 5, Band: "2.4GHz"}, {Channel: 1, Band: "2.4GHz"}}}
	f.Pings["192.168.1.1"] = diagtest.Ping(25, 18, 4)
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
	f := diagtest.Healthy()
	f.Bloat = &speed.Bufferbloat{Idle: diagtest.Ping(12, 1, 0), Loaded: diagtest.Ping(260, 40, 0), IncreaseMs: 248, Grade: "D", DownloadMbps: 100}
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
	f := diagtest.Healthy()
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
	f := diagtest.Healthy()
	f.Pings["1.1.1.1"], f.Pings["8.8.8.8"] = diagtest.Ping(140, 45, 6), diagtest.Ping(150, 50, 8)
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
	f.Pings["192.168.1.1"] = diagtest.Ping(30, 25, 5)
	d = run(t, f, Options{Throughput: true, PlanMbps: 100})
	if d.Primary == KindISP {
		t.Fatalf("ISP blamed while the LAN is unstable: %+v", d.Bottlenecks)
	}
	// No internet at all.
	f = diagtest.Healthy()
	delete(f.Pings, "1.1.1.1")
	delete(f.Pings, "8.8.8.8")
	if d := run(t, f, Options{}); d.Primary != KindISP || !strings.Contains(d.Verdict, "No reply from the internet") {
		t.Fatalf("wan down: %s", d.Verdict)
	}
}

func TestRouterBottleneckOnCable(t *testing.T) {
	f := diagtest.Healthy()
	f.W = nil
	f.Pings["192.168.1.1"] = diagtest.Ping(35, 20, 5)
	d := run(t, f, Options{})
	if d.Primary != KindRouter || d.PrimaryBottleneck().Name != "Router" {
		t.Fatalf("%q %+v", d.Primary, d.Bottlenecks)
	}
}

func TestHotspotNote(t *testing.T) {
	f := diagtest.Healthy()
	f.GW.IP = "172.20.10.1"
	f.Pings["172.20.10.1"] = f.Pings["192.168.1.1"]
	d := run(t, f, Options{})
	if !strings.Contains(d.Notes[0], "phone hotspot") {
		t.Fatalf("notes %v", d.Notes)
	}
}

func TestCompareIsHonest(t *testing.T) {
	before := run(t, diagtest.Healthy(), Options{})
	before.Measurements.Local.DNS[0].MedianMs = 48
	after := run(t, diagtest.Healthy(), Options{})
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

func TestRemeasureAndMissingTest(t *testing.T) {
	f := diagtest.Healthy()
	f.DNSMs["192.168.1.1"] = 80
	before := run(t, f, Options{})
	f.DNSMs["192.168.1.1"] = 9 // as if the system resolver got faster
	after, dl, err := Remeasure(context.Background(), f, "test", before, MetricDNS, 0)
	if err != nil || after == nil || !dl.Improved {
		t.Fatalf("%v %+v", err, dl)
	}
	if MissingTest(MetricLoaded, Options{}) != "--load" || MissingTest(MetricDownload, Options{}) != "--throughput" || MissingTest(MetricDNS, Options{}) != "" {
		t.Fatal("MissingTest")
	}
	if _, err := PlanAuto(&Action{Level: LevelAdvice}); err == nil {
		t.Fatal("PlanAuto must refuse non-auto actions")
	}
}
