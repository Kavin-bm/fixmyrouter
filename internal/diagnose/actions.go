package diagnose

import (
	"fmt"
	"math"
	"strings"

	"github.com/Kavin-bm/fixmyrouter/internal/checks"
	"github.com/Kavin-bm/fixmyrouter/internal/speed"
	"github.com/Kavin-bm/fixmyrouter/profiles"
)

// Fix levels, from most to least automatic.
const (
	LevelAuto   = "auto"   // fixmyrouter changes this computer, after a yes, with snapshot and undo
	LevelRouter = "router" // fixmyrouter changes the router, after a yes and your password, with snapshot and undo (only where a profile allows it)
	LevelGuided = "guided" // exact click-by-click steps for your router model
	LevelAdvice = "advice" // what to do, when we can't do it or don't know your model
)

// Metric keys that a fix is expected to improve (see Compare).
const (
	MetricDNS          = "dns_ms"
	MetricLoaded       = "loaded_latency_ms"
	MetricDownload     = "download_mbps"
	MetricGateway      = "gateway_ms"
	MetricGatewayJit   = "gateway_jitter_ms"
	MetricWAN          = "wan_ms"
	MetricWANLoss      = "wan_loss_pct"
	MetricSignal       = "signal_dbm"
	MetricNeighbours   = "neighbours"
	MetricGatewayLossP = "gateway_loss_pct"
	MetricLinkRate     = "wifi_link_mbps"
)

// Action is one way to fix a bottleneck.
type Action struct {
	ID      string   `json:"id"`
	Level   string   `json:"level"`
	Title   string   `json:"title"`
	Steps   []string `json:"steps,omitempty"`
	Command string   `json:"command,omitempty"` // CLI that does or measures it
	Metric  string   `json:"metric,omitempty"`  // what should improve; checked by fix --measure
	// AutoFix is the local change for LevelAuto.
	AutoFix *checks.AutoFix `json:"auto_fix,omitempty"`
	// RouterSetting and RouterValue are the router change for LevelRouter.
	RouterSetting string `json:"router_setting,omitempty"`
	RouterValue   string `json:"router_value,omitempty"`
}

var levelRank = map[string]int{LevelAuto: 0, LevelRouter: 1, LevelGuided: 2, LevelAdvice: 3}

// Actions maps a bottleneck's issues to fixes, most automatic first.
func Actions(b *Bottleneck, m Measurements, p *profiles.Profile, goos string) []Action {
	var out []Action
	seen := map[string]bool{}
	add := func(a Action) {
		if !seen[a.ID] {
			seen[a.ID] = true
			out = append(out, a)
		}
	}
	r := m.Local
	if r == nil {
		r = &speed.Report{}
	}
	for _, issue := range b.Issues {
		switch issue {
		case "dns-slow", "dns-broken":
			dnsActions(add, r, p, goos)
		case "wifi-channel":
			channelActions(add, r, p)
		case "wifi-band":
			guided(add, p, "band_steering", "wifi-band-steering", "Turn on band steering so devices pick 5 GHz", MetricLinkRate)
			add(Action{ID: "wifi-join-5g", Level: LevelAdvice, Title: "Connect to the 5 GHz network", Metric: MetricLinkRate, Command: "fixmyrouter fix --measure wifi-join-5g", Steps: []string{
				"Open this device's Wi-Fi list and pick the network with 5G or 5GHz in its name (same password as usual).",
				"If there is only one name, your router mixes both bands: turn on band steering, or give the 5 GHz band its own name in the router's wireless settings.",
				"5 GHz is faster and less crowded, but reaches less far through walls: if the signal drops below about -70 dBm, stay on 2.4 GHz in that room.",
			}})
		case "wifi-signal", "wifi-linkrate":
			add(Action{ID: "wifi-placement", Level: LevelAdvice, Title: "Get a stronger signal where you use the internet", Metric: MetricSignal, Steps: []string{
				"Move the router to a central, raised, open spot: not in a cabinet, behind the TV, or next to metal or a fish tank.",
				"Point its antennas straight up (or one up, one sideways).",
				"Work closer to the router, or add a mesh node or access point halfway (not in the dead zone itself).",
				"For a desk, TV or console that never moves, an Ethernet cable beats any Wi-Fi.",
			}, Command: "fixmyrouter fix --measure wifi-placement"})
		case "link-unstable", "gateway-loss", "gateway-latency", "gateway-jitter":
			add(Action{ID: "router-restart", Level: LevelAdvice, Title: "Restart the router, then check its firmware", Metric: MetricGatewayJit, Steps: []string{
				"Unplug the router (and the ISP modem/ONT if separate) for 30 seconds, then plug it back in.",
				"In the router's admin page, look for Firmware or System Update and install any update.",
				"If it is hot to touch, give it air; overheating routers drop packets.",
				"Test with an Ethernet cable: if the cable is fast and steady, the problem is Wi-Fi, not the router.",
			}, Command: "fixmyrouter fix --measure router-restart"})
		case "bufferbloat":
			sqmActions(add, m, p)
		case "wan-down", "wan-loss", "wan-latency", "wan-jitter", "wan-throughput":
			add(Action{ID: "isp-report", Level: LevelAdvice, Title: "Collect evidence and send it to your ISP", Steps: []string{
				"Run `fixmyrouter report --isp --samples 6 --interval 10m` at the time of day it is slowest (it downloads about 25 MB per sample).",
				"Send the Markdown file it writes to your ISP's support with your account number. It shows your own Wi-Fi and router are fine and the slowdown is on their side, with timestamps.",
				"Restart the ISP modem or ONT, and check its cable and lights: a red LOS light means a fibre problem only the ISP can fix.",
				"If it stays below the plan, ask for a line check or a plan downgrade/refund. No software on your side can raise the ISP's speed.",
			}, Command: "fixmyrouter report --isp --samples 6 --interval 10m"})
		}
	}
	// Keep the insertion order within a level, most automatic level first.
	stableSortByLevel(out)
	return out
}

func stableSortByLevel(a []Action) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && levelRank[a[j].Level] < levelRank[a[j-1].Level]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

func modelName(p *profiles.Profile) string {
	return strings.TrimSpace(p.Vendor + " " + p.Model + " " + p.HardwareVersion)
}

// guidedSteps builds click-by-click steps from a profile setting.
func guidedSteps(p *profiles.Profile, key string, final ...string) ([]string, bool) {
	if p == nil {
		return nil, false
	}
	s, ok := p.Settings[key]
	if !ok || s.MenuPath == "n/a" {
		return nil, false
	}
	url := p.Admin.URL
	if url == "" {
		url = "your router's admin page"
	}
	steps := []string{fmt.Sprintf("Open %s in a browser and log in (the password is on the router's sticker if you never changed it).", url)}
	for _, part := range strings.Split(s.MenuPath, " > ") {
		steps = append(steps, "Click "+strings.TrimSpace(part)+".")
	}
	steps = append(steps, final...)
	if s.Notes != "" {
		steps = append(steps, s.Notes)
	}
	steps = append(steps, "Save or Apply. Devices may reconnect for a few seconds.")
	if !p.Verified {
		steps = append(steps, "(These menu names come from the vendor manual and have not been checked on a real "+modelName(p)+" yet.)")
	}
	return steps, true
}

func guided(add func(Action), p *profiles.Profile, key, id, title, metric string, final ...string) {
	if steps, ok := guidedSteps(p, key, final...); ok {
		add(Action{ID: id, Level: LevelGuided, Title: title + " on your " + modelName(p), Steps: steps, Metric: metric,
			Command: "fixmyrouter fix --measure " + id})
	}
}

func dnsActions(add func(Action), r *speed.Report, p *profiles.Profile, goos string) {
	_, best := bestResolver(r.DNS)
	if best == nil {
		return
	}
	secondary := ""
	for _, pr := range speed.PublicResolvers {
		if pr.IP == best.Server {
			secondary = pr.Secondary
		}
	}
	servers := strings.TrimSuffix(best.Server+", "+secondary, ", ")
	if goos == "darwin" && secondary != "" {
		add(Action{ID: "set-dns", Level: LevelAuto, Metric: MetricDNS,
			Title:   fmt.Sprintf("Switch this Mac's DNS to %s (%s), the fastest we measured", best.Name, servers),
			Command: "fixmyrouter fix --measure set-dns",
			Steps:   []string{"Saves your current DNS setting, switches to " + servers + ", checks it took, then measures again. `fixmyrouter undo` puts the old setting back."},
			AutoFix: &checks.AutoFix{ID: "set-dns", Args: []string{best.Server, secondary},
				Summary: fmt.Sprintf("Switch this computer's DNS to %s (%s). Other devices are not changed.", best.Name, servers)},
		})
	} else {
		add(Action{ID: "set-dns-device", Level: LevelAdvice, Metric: MetricDNS,
			Title: fmt.Sprintf("Set this device's DNS to %s (%s)", best.Name, servers),
			Steps: []string{"Automatic DNS change is macOS-only for now. In this device's network settings, set DNS servers to " + servers + "."}})
	}
	guided(add, p, "dns_servers", "router-dns", "Give every device the faster DNS", MetricDNS, "Set the primary DNS to "+best.Server+" and the secondary to "+secondary+".")
	if p == nil {
		add(Action{ID: "router-dns-advice", Level: LevelAdvice, Metric: MetricDNS,
			Title: "Give every device the faster DNS from the router",
			Steps: []string{"In the router's admin page, find DHCP or LAN settings, set the DNS servers to " + servers + ", and save. Phones, TVs and laptops pick it up when they reconnect."}})
	}
}

// routerNote warns when a router fix comes from a profile nobody has tested on a real device.
func routerNote(p *profiles.Profile) []string {
	if p != nil && !p.Verified {
		return []string{"Unverified profile: the " + modelName(p) + " driver is tested against a simulated router only, not a real device yet. You can undo it, but check the result."}
	}
	return nil
}

func channelActions(add func(Action), r *speed.Report, p *profiles.Profile) {
	c := r.Congestion
	if c == nil || c.BestChannel == 0 {
		return
	}
	key := "wifi_channel_5g"
	if c.BestChannel <= 14 {
		key = "wifi_channel_2g"
	}
	val := fmt.Sprint(c.BestChannel)
	title := fmt.Sprintf("Move your %s Wi-Fi to channel %d", c.Band, c.BestChannel)
	if p.CanRouterFix(key) {
		add(Action{ID: "router-" + key, Level: LevelRouter, Title: title + " (fixmyrouter does it on the router)", Metric: MetricNeighbours,
			RouterSetting: key, RouterValue: val, Command: "fixmyrouter router apply " + key + " " + val,
			Steps: append([]string{"Shows the exact change first, asks for your router admin password (used once, never stored), saves a snapshot, then changes the channel. `fixmyrouter router undo` restores it."}, routerNote(p)...)})
	}
	guided(add, p, key, "guided-"+key, title, MetricNeighbours, fmt.Sprintf("Choose channel %d instead of Auto.", c.BestChannel))
	add(Action{ID: "channel-advice", Level: LevelAdvice, Title: title, Metric: MetricNeighbours, Command: "fixmyrouter fix --measure channel-advice", Steps: []string{
		fmt.Sprintf("Open the router's admin page, go to Wireless (or Wi-Fi) settings for %s.", c.Band),
		fmt.Sprintf("Set Channel to %d instead of Auto, and save.", c.BestChannel),
		fmt.Sprintf("Why %d: it is shared with %d nearby networks, yours with %d.", c.BestChannel, c.BestCount, c.OnMyChannel),
	}})
}

func sqmActions(add func(Action), m Measurements, p *profiles.Profile) {
	mbps := downloadMbps(m)
	target := math.Floor(mbps * 0.9)
	rate := "about 90% of your plan's download and upload speed"
	if target >= 1 {
		rate = fmt.Sprintf("download about %.0f Mbps (90%% of the %.0f Mbps we measured) and upload about 90%% of your plan's upload", target, mbps)
	}
	if p.CanRouterFix("sqm_qos") && target >= 1 {
		val := fmt.Sprintf("%.0f", target)
		add(Action{ID: "router-sqm_qos", Level: LevelRouter, Title: "Turn on SQM (smart queue) on the router", Metric: MetricLoaded,
			RouterSetting: "sqm_qos", RouterValue: val, Command: "fixmyrouter router apply sqm_qos " + val + "/UPLOAD_MBPS",
			Steps: append([]string{"Replace UPLOAD_MBPS with about 90% of your plan's upload (or leave it as " + val + " to shape download only).", "Shows the exact change first, asks for your router admin password (used once, never stored), saves a snapshot. `fixmyrouter router undo` restores it."}, routerNote(p)...)})
	}
	guided(add, p, "sqm_qos", "guided-sqm", "Turn on QoS / smart queue", MetricLoaded, "Turn it on and set "+rate+".")
	add(Action{ID: "sqm-advice", Level: LevelAdvice, Title: "Turn on SQM, Smart Queue or Adaptive QoS", Metric: MetricLoaded, Command: "fixmyrouter fix --measure sqm-advice --load", Steps: []string{
		"In the router's admin page look for SQM, Smart Queue, QoS, Adaptive QoS or Bandwidth Control.",
		"Turn it on and set " + rate + ". Prefer fq_codel or cake if you get a choice.",
		"If the router has no such option, the line will keep lagging under load: routers running OpenWrt, or ones with cake/fq_codel, fix it properly.",
	}})
}
