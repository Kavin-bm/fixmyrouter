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
	cur, sec, near := ParseNmcli("*:36:5180 MHz:80:WPA2\n :6:2437 MHz:40:WPA1 WPA2\n :149:5745 MHz:30:\n")
	if cur.Channel != 36 || sec != "wpa2" || len(near) != 2 || near[0].Band != "2.4GHz" {
		t.Fatalf("%+v %q %+v", cur, sec, near)
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

func TestFindings(t *testing.T) {
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
	for _, f := range Findings(r) {
		got[f.Title] = true
		if f.ID == "dns-speed" && (f.AutoFix == nil || f.AutoFix.Args[1] != "1.0.0.1") {
			t.Errorf("dns finding should offer set-dns: %+v", f.AutoFix)
		}
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

func TestSecurityMapping(t *testing.T) {
	for in, want := range map[string]string{
		"spairport_security_mode_none": "open", "spairport_security_mode_wpa2_personal": "wpa2",
		"spairport_security_mode_wpa2_personal_mixed": "wpa-wpa2", "spairport_security_mode_wpa3_transition": "wpa2-wpa3",
		"spairport_security_mode_wpa3_personal": "wpa3", "spairport_security_mode_wep": "wep",
		"spairport_security_mode_wpa2_enterprise": "enterprise",
	} {
		if got := macSecurity(in); got != want {
			t.Errorf("mac %s -> %s", in, got)
		}
	}
	for in, want := range map[string]string{"": "open", "WEP": "wep", "WPA1": "wpa", "WPA2 WPA3": "wpa2-wpa3", "WPA2 802.1X": "enterprise"} {
		if got := nmcliSecurity(in); got != want {
			t.Errorf("nmcli %q -> %s", in, got)
		}
	}
}

const netshIf = "\r\nThere is 1 interface on the system:\r\n\r\n" +
	"    Name                   : Wi-Fi\r\n" +
	"    State                  : connected\r\n" +
	"    SSID                   : Home\r\n" +
	"    AP BSSID               : 9c:53:22:aa:bb:01\r\n" +
	"    Band                   : 2.4 GHz\r\n" +
	"    Channel                : 6\r\n" +
	"    Radio type             : 802.11n\r\n" +
	"    Authentication         : WPA2-Personal\r\n" +
	"    Cipher                 : CCMP\r\n" +
	"    Receive rate (Mbps)    : 144.4\r\n" +
	"    Signal                 : 60%\r\n"

const netshNets = "SSID 1 : Home\r\n    Network type            : Infrastructure\r\n" +
	"    BSSID 1                 : 9c:53:22:aa:bb:01\r\n         Signal             : 60%\r\n         Channel            : 6\r\n" +
	"SSID 2 : Neighbour\r\n    BSSID 1                 : 00:11:22:33:44:55\r\n         Channel            : 1\r\n" +
	"    BSSID 2                 : 00:11:22:33:44:56\r\n         Channel            : 36\r\n"

func TestNetsh(t *testing.T) {
	w, ok := ParseNetshInterfaces(netshIf)
	if !ok || w.Channel != 6 || w.Band != "2.4GHz" || w.SignalDBm != -70 || w.Security != "wpa2" || w.TxRateMbps != 144 || w.bssid != "9c:53:22:aa:bb:01" {
		t.Fatalf("%+v ok=%v", w, ok)
	}
	near := ParseNetshNetworks(netshNets, w.bssid)
	if len(near) != 2 || near[0].Channel != 1 || near[1].Band != "5GHz" {
		t.Fatalf("own AP must be skipped: %+v", near)
	}
	if _, ok := ParseNetshInterfaces("There is 1 interface on the system:\r\n    State : disconnected\r\n"); ok {
		t.Fatal("disconnected should not be ok")
	}
}

func TestWindowsPingParse(t *testing.T) {
	out := "Reply from 192.168.1.1: bytes=32 time=3ms TTL=64\r\nReply from 192.168.1.1: bytes=32 time<1ms TTL=64\r\nRequest timed out.\r\n"
	if s := ParsePing(out, "192.168.1.1", 3); s.Received != 2 || s.MaxMs != 3 {
		t.Fatalf("%+v", s)
	}
}
