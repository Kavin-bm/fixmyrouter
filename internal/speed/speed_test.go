package speed

import "testing"

const spSample = `{"SPAirPortDataType":[{"spairport_airport_interfaces":[{"_name":"en0",
"spairport_current_network_information":{"_name":"<redacted>","spairport_network_channel":"6 (2GHz, 20MHz)","spairport_network_phymode":"802.11n","spairport_network_rate":144,"spairport_signal_noise":"-72 dBm / -93 dBm"},
"spairport_supported_channels":["1 (2GHz)","36 (5GHz)"],
"spairport_airport_other_local_wireless_networks":[
{"spairport_network_channel":"6 (2GHz, 20MHz)"},{"spairport_network_channel":"5 (2GHz, 20MHz)"},{"spairport_network_channel":"8 (2GHz, 40MHz)"},
{"spairport_network_channel":"11 (2GHz, 20MHz)"},{"spairport_network_channel":"149 (5GHz, 80MHz)"}]},
{"_name":"awdl0","spairport_current_network_information":{}}]}]}`

func TestParseSPAirPort(t *testing.T) {
	w, ok := ParseSPAirPort([]byte(spSample))
	if !ok || w.Channel != 6 || w.Band != "2.4GHz" || w.WidthMHz != 20 || w.SignalDBm != -72 || w.NoiseDBm != -93 || !w.Supports5GHz || w.TxRateMbps != 144 {
		t.Fatalf("%+v ok=%v", w, ok)
	}
	if len(w.NearbyChannels) != 5 {
		t.Fatalf("nearby %d", len(w.NearbyChannels))
	}
	c, ok := Analyze(w)
	if !ok || c.OnMyChannel != 3 || c.BestChannel != 1 || c.BestCount != 1 || c.Seen != 4 {
		t.Fatalf("congestion %+v", c)
	}
}

func TestFreqAndIw(t *testing.T) {
	cases := map[int][2]any{2412: {1, "2.4GHz"}, 2437: {6, "2.4GHz"}, 5180: {36, "5GHz"}, 5745: {149, "5GHz"}, 5955: {1, "6GHz"}}
	for f, want := range cases {
		ch, b := ChannelFromFreq(f)
		if ch != want[0].(int) || b != want[1].(string) {
			t.Errorf("%d -> %d %s", f, ch, b)
		}
	}
	w, ok := ParseIwLink("Connected to aa:bb:cc:dd:ee:ff (on wlan0)\n\tSSID: x\n\tfreq: 5180.0\n\tsignal: -55 dBm\n\ttx bitrate: 866.7 MBit/s\n")
	if !ok || w.Channel != 36 || w.SignalDBm != -55 || w.TxRateMbps != 866 {
		t.Fatalf("%+v", w)
	}
	if got := ParseIwDevInterfaces("phy#0\n\tInterface wlan0\n\t\ttype managed\n"); len(got) != 1 || got[0] != "wlan0" {
		t.Fatalf("%v", got)
	}
	cur, near := ParseNmcli("*:36:5180 MHz:80\n :6:2437 MHz:40\n :149:5745 MHz:30\n")
	if cur.Channel != 36 || len(near) != 2 || near[0].Band != "2.4GHz" {
		t.Fatalf("%+v %+v", cur, near)
	}
}

func TestPingParse(t *testing.T) {
	out := "64 bytes from 192.168.1.1: icmp_seq=0 ttl=64 time=2.0 ms\n64 bytes from 192.168.1.1: icmp_seq=1 ttl=64 time=6.0 ms\n64 bytes from 192.168.1.1: icmp_seq=3 ttl=64 time=4.0 ms\n"
	s := ParsePing(out, "192.168.1.1", 4)
	if s.Received != 3 || s.LossPct != 25 || s.MedianMs != 4 || s.MinMs != 2 || s.MaxMs != 6 || s.JitterMs != 3 {
		t.Fatalf("%+v", s)
	}
}

func TestDNSParsers(t *testing.T) {
	sc := "resolver #1\n  nameserver[0] : 192.168.1.1\n  nameserver[1] : 8.8.8.8\nresolver #2\n  nameserver[0] : 192.168.1.1\n"
	if got := ParseScutilDNS(sc); len(got) != 2 || got[0] != "192.168.1.1" {
		t.Fatalf("%v", got)
	}
	if got := ParseResolvConf("# x\nnameserver 127.0.0.53\noptions edns0\n"); len(got) != 1 || got[0] != "127.0.0.53" {
		t.Fatalf("%v", got)
	}
}

func TestBloatGrade(t *testing.T) {
	for in, want := range map[float64]string{2: "A+", 20: "A", 45: "B", 150: "C", 300: "D", 900: "F"} {
		if got := BloatGrade(in); got != want {
			t.Errorf("%v -> %s", in, got)
		}
	}
}

func TestSuggest(t *testing.T) {
	w, _ := ParseSPAirPort([]byte(spSample))
	c, _ := Analyze(w)
	r := &Report{
		WiFi: &w, Congestion: &c,
		Gateway: &PingStats{Received: 20, Sent: 20, MedianMs: 3, JitterMs: 1},
		DNS: []DNSResult{
			{Name: "Your current DNS", Server: "192.168.1.1", MedianMs: 80, Queries: 15, System: true},
			{Name: "Cloudflare", Server: "1.1.1.1", MedianMs: 12, Queries: 15},
		},
	}
	got := map[string]bool{}
	for _, s := range Suggest(r) {
		got[s.Title] = true
	}
	for _, want := range []string{"Use 5 GHz Wi-Fi", "Improve Wi-Fi signal", "Switch Wi-Fi channel to 1", "Change DNS to Cloudflare (1.1.1.1)"} {
		if !got[want] {
			t.Errorf("missing suggestion %q in %v", want, got)
		}
	}
	if got["Your Wi-Fi link to the router is unstable"] {
		t.Error("gateway is healthy")
	}
}
