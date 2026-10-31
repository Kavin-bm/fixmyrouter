package diagnose

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Kavin-bm/fixmyrouter/internal/discover"
	"github.com/Kavin-bm/fixmyrouter/internal/speed"
	"github.com/Kavin-bm/fixmyrouter/internal/store"
	"github.com/Kavin-bm/fixmyrouter/profiles"
)

// Bottleneck kinds.
const (
	KindWiFi        = "wifi"
	KindBufferbloat = "bufferbloat"
	KindDNS         = "dns"
	KindRouter      = "router"
	KindISP         = "isp"
)

// MinImpact is the impact from which a bottleneck counts as real.
const MinImpact = 20

// WAN is what we measured beyond the router.
type WAN struct {
	Pings      []speed.PingStats `json:"pings"`
	Throughput *speed.Throughput `json:"throughput,omitempty"`
	Note       string            `json:"note,omitempty"`
}

// Measurements are the raw numbers a diagnosis is based on.
type Measurements struct {
	Gateway *discover.Gateway `json:"gateway"`
	Local   *speed.Report     `json:"local"`
	WAN     WAN               `json:"wan"`
}

// Evidence is one number behind a verdict.
type Evidence struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Note  string `json:"note,omitempty"`
}

// Bottleneck is one reason the connection is slow.
type Bottleneck struct {
	Kind     string     `json:"kind"`
	Name     string     `json:"name"`
	Impact   int        `json:"impact"` // 0-100, how much it likely hurts
	Headline string     `json:"headline"`
	Explain  string     `json:"explain"`
	Issues   []string   `json:"issues"`
	Evidence []Evidence `json:"evidence"`
	Actions  []Action   `json:"actions"`
}

// Diagnosis is the result of `fixmyrouter diagnose`.
type Diagnosis struct {
	Tool         string       `json:"tool"`
	Version      string       `json:"version"`
	GeneratedAt  time.Time    `json:"generated_at"`
	Options      Options      `json:"options"`
	Profile      string       `json:"router_profile,omitempty"`
	Verdict      string       `json:"verdict"`
	Primary      string       `json:"primary,omitempty"` // kind of the main bottleneck, "" if none
	Healthy      bool         `json:"healthy"`
	Bottlenecks  []Bottleneck `json:"bottlenecks"`
	Notes        []string     `json:"notes,omitempty"`
	Measurements Measurements `json:"measurements"`
}

// PrimaryBottleneck returns the main bottleneck, or nil.
func (d *Diagnosis) PrimaryBottleneck() *Bottleneck {
	for i := range d.Bottlenecks {
		if d.Bottlenecks[i].Kind == d.Primary {
			return &d.Bottlenecks[i]
		}
	}
	return nil
}

// FindAction returns the action with this ID across all bottlenecks.
func (d *Diagnosis) FindAction(id string) (*Action, *Bottleneck) {
	for i := range d.Bottlenecks {
		for j := range d.Bottlenecks[i].Actions {
			if d.Bottlenecks[i].Actions[j].ID == id {
				return &d.Bottlenecks[i].Actions[j], &d.Bottlenecks[i]
			}
		}
	}
	return nil, nil
}

// Run measures, matches a router profile, and analyses.
func Run(ctx context.Context, p Prober, version string, opt Options) (*Diagnosis, error) {
	m, err := Collect(ctx, p, opt)
	if err != nil {
		return nil, err
	}
	var prof *profiles.Profile
	if m.Gateway != nil {
		prof = profiles.Find(m.Gateway.HTTPTitle, m.Gateway.HTTPServer)
	}
	d := Analyze(m, opt, prof, p.GOOS())
	d.Tool, d.Version = "fixmyrouter", version
	return d, nil
}

// Analyze turns measurements into a verdict. It is pure, for tests.
func Analyze(m Measurements, opt Options, prof *profiles.Profile, goos string) *Diagnosis {
	d := &Diagnosis{GeneratedAt: time.Now().UTC(), Options: opt, Measurements: m, Bottlenecks: []Bottleneck{}}
	if prof != nil {
		d.Profile = prof.File
	}
	r := m.Local
	if r == nil {
		r = &speed.Report{}
	}
	wifi := wifiBottleneck(r)
	link := linkBottleneck(r, wifi)
	for _, b := range []*Bottleneck{wifi, link, bloatBottleneck(r), ispBottleneck(m, opt, wifi != nil || link != nil), dnsBottleneck(r)} {
		if b != nil {
			b.Actions = Actions(b, m, prof, goos)
			d.Bottlenecks = append(d.Bottlenecks, *b)
		}
	}
	order := map[string]int{KindWiFi: 0, KindBufferbloat: 1, KindISP: 2, KindRouter: 3, KindDNS: 4}
	sort.SliceStable(d.Bottlenecks, func(i, j int) bool {
		a, b := d.Bottlenecks[i], d.Bottlenecks[j]
		if a.Impact != b.Impact {
			return a.Impact > b.Impact
		}
		return order[a.Kind] < order[b.Kind]
	})
	if len(d.Bottlenecks) > 0 && d.Bottlenecks[0].Impact >= MinImpact {
		d.Primary = d.Bottlenecks[0].Kind
	}
	d.Healthy = d.Primary == ""
	d.Notes = notes(m, opt)
	d.Verdict = verdict(d)
	return d
}

func verdict(d *Diagnosis) string {
	if d.Healthy {
		v := "No bottleneck found in what we measured: Wi-Fi, router, DNS and the internet path look healthy."
		if !d.Options.Load || !d.Options.Throughput {
			v += " Run again with the download tests to also check lag under load and real download speed."
		}
		return v
	}
	p := d.PrimaryBottleneck()
	v := fmt.Sprintf("Main bottleneck: %s. %s", p.Name, p.Headline)
	var also []string
	for _, b := range d.Bottlenecks {
		if b.Kind != p.Kind && b.Impact >= MinImpact {
			also = append(also, strings.ToLower(b.Name[:1])+b.Name[1:])
		}
	}
	if len(also) > 0 {
		v += " Also worth fixing: " + strings.Join(also, ", ") + "."
	}
	return v
}

// IsHotspot reports a phone hotspot gateway (iPhone 172.20.10.1, Android
// 192.168.43.1), where router fixes don't apply.
func IsHotspot(ip string) bool {
	return strings.HasPrefix(ip, "172.20.10.") || ip == "192.168.43.1"
}

func notes(m Measurements, opt Options) []string {
	var n []string
	if m.Gateway != nil && IsHotspot(m.Gateway.IP) {
		n = append(n, "This looks like a phone hotspot ("+m.Gateway.IP+"), not a home router: router fixes don't apply, and the \"ISP\" here is the mobile network.")
	}
	if m.Local != nil && m.Local.Gateway == nil {
		n = append(n, "The router did not answer ping (some block it), so router health is unknown.")
	}
	if !opt.Load {
		n = append(n, "Lag under load (bufferbloat) was not tested: add --load (downloads up to 200 MB).")
	} else if m.Local != nil && m.Local.BloatNote != "" {
		n = append(n, m.Local.BloatNote)
	}
	if !opt.Throughput {
		n = append(n, fmt.Sprintf("Internet download speed was not tested: add --throughput (downloads up to %d MB).", capMB(opt)))
	} else if m.WAN.Note != "" {
		n = append(n, m.WAN.Note)
	}
	if opt.PlanMbps == 0 && downloadMbps(m) > 0 {
		n = append(n, "Add --plan <Mbps> (your plan's download speed) to compare the measured speed with what you pay for.")
	}
	return n
}

func capMB(opt Options) int {
	if opt.ThroughputMB > 0 {
		return opt.ThroughputMB
	}
	return speed.DefaultThroughputMB
}

// downloadMbps is the measured download speed: the throughput test, else the
// bufferbloat test's download (single stream, so a lower bound).
func downloadMbps(m Measurements) float64 {
	if t := m.WAN.Throughput; t != nil {
		return t.DownloadMbps
	}
	if m.Local != nil && m.Local.Bufferbloat != nil {
		return m.Local.Bufferbloat.DownloadMbps
	}
	return 0
}

func ev(name, format string, a ...any) Evidence {
	return Evidence{Name: name, Value: fmt.Sprintf(format, a...)}
}

func wifiBottleneck(r *speed.Report) *Bottleneck {
	w := r.WiFi
	if w == nil {
		return nil
	}
	b := &Bottleneck{Kind: KindWiFi, Name: "Wi-Fi"}
	var heads []string
	add := func(issue string, impact int, head string) {
		b.Issues = append(b.Issues, issue)
		b.Impact = max(b.Impact, impact)
		heads = append(heads, head)
	}
	switch q := speed.SignalQuality(w.SignalDBm); q {
	case "weak":
		add("wifi-signal", 70, fmt.Sprintf("Weak signal (%d dBm)", w.SignalDBm))
	case "fair":
		add("wifi-signal", 40, fmt.Sprintf("Only fair signal (%d dBm)", w.SignalDBm))
	}
	if w.SignalDBm != 0 {
		b.Evidence = append(b.Evidence, ev("Signal", "%d dBm (%s; good is -67 or better)", w.SignalDBm, speed.SignalQuality(w.SignalDBm)))
	}
	if w.Band == "2.4GHz" && w.Supports5GHz {
		add("wifi-band", 45, "on 2.4 GHz although this device supports 5 GHz")
	}
	b.Evidence = append(b.Evidence, ev("Band and channel", "%s, channel %d", w.Band, w.Channel))
	if c := r.Congestion; c != nil && c.BestChannel != 0 {
		b.Evidence = append(b.Evidence, ev("Neighbours on your channel", "%d (channel %d has %d)", c.OnMyChannel, c.BestChannel, c.BestCount))
		if diff := c.OnMyChannel - c.BestCount; diff >= 2 {
			add("wifi-channel", min(50, 30+5*(diff-2)), fmt.Sprintf("crowded channel (%d neighbours; channel %d has %d)", c.OnMyChannel, c.BestChannel, c.BestCount))
		}
	}
	if w.TxRateMbps > 0 {
		b.Evidence = append(b.Evidence, ev("Link rate", "%d Mbps", w.TxRateMbps))
		if w.TxRateMbps < 100 {
			add("wifi-linkrate", 35, fmt.Sprintf("slow link rate (%d Mbps)", w.TxRateMbps))
		}
	}
	if len(b.Issues) == 0 {
		return nil
	}
	if len(b.Issues) > 1 {
		b.Impact = min(90, b.Impact+10)
	}
	heads[0] = strings.ToUpper(heads[0][:1]) + heads[0][1:]
	b.Headline = strings.Join(heads, ", ") + "."
	b.Explain = "Your connection is slowed down between this device and the router, before the internet is even involved. A faster plan would not help; better Wi-Fi will."
	return b
}

// linkBottleneck looks at ping to the router. On Wi-Fi with a Wi-Fi problem
// it becomes extra evidence for Wi-Fi; otherwise it is its own bottleneck.
func linkBottleneck(r *speed.Report, wifi *Bottleneck) *Bottleneck {
	g := r.Gateway
	if g == nil || g.Received == 0 {
		return nil
	}
	impact := 0
	var issues []string
	if g.LossPct > 2 {
		impact, issues = max(impact, 60), append(issues, "gateway-loss")
	}
	if g.MedianMs > 20 {
		impact, issues = max(impact, 45), append(issues, "gateway-latency")
	}
	if g.JitterMs > 10 {
		impact, issues = max(impact, 35), append(issues, "gateway-jitter")
	}
	e := ev("Ping to router", "median %.1f ms, jitter %.1f ms, loss %.0f%% (healthy: under 5 ms, little jitter, no loss)", g.MedianMs, g.JitterMs, g.LossPct)
	if impact == 0 {
		if wifi != nil {
			wifi.Evidence = append(wifi.Evidence, e)
		}
		return nil
	}
	if wifi != nil {
		wifi.Evidence = append(wifi.Evidence, e)
		wifi.Issues = append(wifi.Issues, "link-unstable")
		wifi.Impact = min(90, max(wifi.Impact, impact)+5)
		wifi.Headline = strings.TrimSuffix(wifi.Headline, ".") + fmt.Sprintf(", and the link to the router is unstable (%.0f ms jitter, %.0f%% loss).", g.JitterMs, g.LossPct)
		return nil
	}
	b := &Bottleneck{Kind: KindRouter, Name: "Router", Impact: impact, Issues: issues, Evidence: []Evidence{e}}
	b.Headline = fmt.Sprintf("Even the hop to your router is slow or unsteady (median %.1f ms, jitter %.1f ms, %.0f%% loss).", g.MedianMs, g.JitterMs, g.LossPct)
	if r.WiFi != nil {
		b.Name = "Router or the Wi-Fi link to it"
		b.Explain = "The Wi-Fi signal itself looks fine, so the router may be overloaded, or another device is hogging the air. A test over a cable tells which."
	} else {
		b.Explain = "On a cable this hop should be under 1-2 ms with no loss. An overloaded, overheating or outdated router is the usual cause."
	}
	return b
}

func bloatBottleneck(r *speed.Report) *Bottleneck {
	bl := r.Bufferbloat
	if bl == nil || bl.IncreaseMs < 30 {
		return nil
	}
	impact := 20
	switch {
	case bl.IncreaseMs >= 200:
		impact = 80
	case bl.IncreaseMs >= 60:
		impact = 60
	}
	return &Bottleneck{
		Kind: KindBufferbloat, Name: "Bufferbloat (lag when the line is busy)", Impact: impact, Issues: []string{"bufferbloat"},
		Headline: fmt.Sprintf("Latency jumps by %.0f ms when the line is busy (grade %s).", bl.IncreaseMs, bl.Grade),
		Explain:  "When anyone downloads, uploads or streams, the router's queue fills up and every other request waits behind it. Speed tests look fine, but calls, games and page loads stutter. Smart queue management (SQM/QoS) on the router fixes it.",
		Evidence: []Evidence{
			ev("Latency idle", "%.1f ms", bl.Idle.MedianMs),
			ev("Latency while downloading", "%.1f ms (+%.0f ms)", bl.Loaded.MedianMs, bl.IncreaseMs),
			ev("Download during test", "%.0f Mbps", bl.DownloadMbps),
		},
	}
}

// BestWAN returns the internet ping with the lowest loss, then lowest median.
func BestWAN(pings []speed.PingStats) *speed.PingStats {
	var best *speed.PingStats
	for i := range pings {
		p := &pings[i]
		if p.Received == 0 {
			continue
		}
		if best == nil || p.LossPct < best.LossPct || (p.LossPct == best.LossPct && p.MedianMs < best.MedianMs) {
			best = p
		}
	}
	return best
}

func ispBottleneck(m Measurements, opt Options, localProblem bool) *Bottleneck {
	if len(m.WAN.Pings) == 0 && m.WAN.Throughput == nil {
		return nil
	}
	b := &Bottleneck{Kind: KindISP, Name: "Internet provider (beyond your router)"}
	var heads []string
	add := func(issue string, impact int, head string) {
		b.Issues = append(b.Issues, issue)
		b.Impact = max(b.Impact, impact)
		heads = append(heads, head)
	}
	var gw *speed.PingStats
	if m.Local != nil {
		gw = m.Local.Gateway
	}
	w := BestWAN(m.WAN.Pings)
	if len(m.WAN.Pings) > 0 && w == nil {
		add("wan-down", 90, "no reply from the internet at all")
		b.Evidence = append(b.Evidence, ev("Internet ping", "no reply from %s", strings.Join(WANTargets, " or ")))
	}
	if w != nil {
		b.Evidence = append(b.Evidence, ev("Internet ping ("+w.Target+")", "median %.1f ms, jitter %.1f ms, loss %.0f%%", w.MedianMs, w.JitterMs, w.LossPct))
		added := w.MedianMs
		if gw != nil {
			added = w.MedianMs - gw.MedianMs
			b.Evidence = append(b.Evidence, ev("Added beyond the router", "%.1f ms", added))
		}
		if w.LossPct > 2 {
			add("wan-loss", 60, fmt.Sprintf("%.0f%% packet loss to the internet", w.LossPct))
		}
		if added > 80 {
			add("wan-latency", 40, fmt.Sprintf("%.0f ms added beyond your router", added))
		}
		if w.JitterMs > 30 {
			add("wan-jitter", 35, fmt.Sprintf("unsteady internet latency (%.0f ms jitter)", w.JitterMs))
		}
	}
	if mbps := downloadMbps(m); mbps > 0 {
		src := "download test"
		if m.WAN.Throughput == nil {
			src = "bufferbloat test (single download, a lower bound)"
		}
		b.Evidence = append(b.Evidence, ev("Download speed", "%.0f Mbps (%s)", mbps, src))
		switch {
		case opt.PlanMbps > 0 && mbps < 0.5*opt.PlanMbps:
			add("wan-throughput", 70, fmt.Sprintf("download %.0f Mbps is under half of your %.0f Mbps plan", mbps, opt.PlanMbps))
		case opt.PlanMbps > 0 && mbps < 0.8*opt.PlanMbps:
			add("wan-throughput", 40, fmt.Sprintf("download %.0f Mbps is below 80%% of your %.0f Mbps plan", mbps, opt.PlanMbps))
		case opt.PlanMbps == 0 && mbps < 10:
			add("wan-throughput", 45, fmt.Sprintf("download only %.0f Mbps (HD video needs about 5-10 per stream)", mbps))
		}
		if opt.PlanMbps > 0 {
			b.Evidence = append(b.Evidence, ev("Your plan", "%.0f Mbps (%.0f%% delivered)", opt.PlanMbps, 100*mbps/opt.PlanMbps))
		}
	}
	if len(b.Issues) == 0 {
		return nil
	}
	gwOK := gw == nil || (gw.LossPct <= 1 && gw.MedianMs <= 15)
	b.Explain = "Your side looks fine: the router answers quickly, but things get slow or lossy beyond it. That is the ISP's line or network. Nothing on your side can raise it, but an evidence report helps the ISP fix it."
	if !gwOK || localProblem {
		b.Impact /= 2
		b.Explain = "The internet path looks poor, but your own Wi-Fi or router link is also unsteady, so fix that first and measure again before blaming the ISP."
	}
	heads[0] = strings.ToUpper(heads[0][:1]) + heads[0][1:]
	b.Headline = strings.Join(heads, ", ") + "."
	return b
}

// bestResolver picks the fastest public resolver that answered reliably.
func bestResolver(results []speed.DNSResult) (sys, best *speed.DNSResult) {
	for i := range results {
		d := &results[i]
		if d.System {
			sys = d
			continue
		}
		if d.Queries == 0 || d.Failures*5 > d.Queries { // more than 20% failed: unreliable
			continue
		}
		if best == nil || d.MedianMs < best.MedianMs {
			best = d
		}
	}
	return sys, best
}

func dnsBottleneck(r *speed.Report) *Bottleneck {
	sys, best := bestResolver(r.DNS)
	if sys == nil || best == nil {
		return nil
	}
	b := &Bottleneck{Kind: KindDNS, Name: "DNS (looking up website names)"}
	broken := sys.Queries > 0 && sys.Failures == sys.Queries
	gap := sys.MedianMs - best.MedianMs
	switch {
	case broken:
		b.Impact, b.Issues = 70, []string{"dns-broken"}
		b.Headline = fmt.Sprintf("Your DNS (%s) did not answer; %s answered in %.0f ms.", sys.Server, best.Name, best.MedianMs)
	case gap >= 15 && best.MedianMs < sys.MedianMs*0.7:
		b.Impact, b.Issues = 25+min(30, int(gap/5)), []string{"dns-slow"}
		if sys.MedianMs > 150 {
			b.Impact = max(b.Impact, 45)
		}
		b.Headline = fmt.Sprintf("Each new website waits %.0f ms for your DNS; %s answers in %.0f ms.", sys.MedianMs, best.Name, best.MedianMs)
	default:
		return nil
	}
	b.Explain = "Before a page loads, its name is looked up. A slow resolver adds that delay to every new site and app. Switching resolver is safe, free and reversible."
	for _, d := range r.DNS {
		v := fmt.Sprintf("%.1f ms", d.MedianMs)
		if d.Failures == d.Queries {
			v = "no answer"
		} else if d.Failures > 0 {
			v += fmt.Sprintf(" (%d/%d failed)", d.Failures, d.Queries)
		}
		b.Evidence = append(b.Evidence, Evidence{Name: d.Name + " (" + d.Server + ")", Value: v})
	}
	return b
}

const diagnosesFile = "diagnoses.json"
const keepDiagnoses = 20

// Save appends a diagnosis to the local history.
func Save(d *Diagnosis) error {
	all, err := History()
	if err != nil {
		return err
	}
	all = append(all, *d)
	if len(all) > keepDiagnoses {
		all = all[len(all)-keepDiagnoses:]
	}
	return store.Save(diagnosesFile, all)
}

// History returns saved diagnoses, oldest first.
func History() ([]Diagnosis, error) {
	var all []Diagnosis
	err := store.Load(diagnosesFile, &all)
	return all, err
}

// Latest returns the most recent saved diagnosis, or nil.
func Latest() *Diagnosis {
	all, _ := History()
	if len(all) == 0 {
		return nil
	}
	return &all[len(all)-1]
}
