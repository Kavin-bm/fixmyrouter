package discover

import "testing"

func TestParseRouteGet(t *testing.T) {
	out := `   route to: default
destination: default
       mask: default
    gateway: 192.168.1.1
  interface: en0
      flags: <UP,GATEWAY,DONE,STATIC,PRCLONING,GLOBAL>`
	ip, iface := ParseRouteGet(out)
	if ip != "192.168.1.1" || iface != "en0" {
		t.Fatalf("got %q %q", ip, iface)
	}
}

func TestParseProcNetRoute(t *testing.T) {
	data := "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n" +
		"wlan0\t00000000\t0101A8C0\t0003\t0\t0\t600\t00000000\t0\t0\t0\n" +
		"eth0\t00000000\t0100000A\t0003\t0\t0\t100\t00000000\t0\t0\t0\n" +
		"eth0\t0000000A\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0\n"
	ip, iface := ParseProcNetRoute(data)
	if ip != "10.0.0.1" || iface != "eth0" {
		t.Fatalf("got %q %q", ip, iface)
	}
}

func TestParseArp(t *testing.T) {
	if got := ParseArpDarwin("? (192.168.1.1) at a4:2b:b0:1:2:3 on en0 ifscope [ethernet]", "192.168.1.1"); got != "a4:2b:b0:01:02:03" {
		t.Fatalf("darwin: %q", got)
	}
	linux := "IP address       HW type     Flags       HW address            Mask     Device\n192.168.1.1      0x1         0x2         a4:2b:b0:aa:bb:cc     *        wlan0\n"
	if got := ParseProcNetArp(linux, "192.168.1.1"); got != "a4:2b:b0:aa:bb:cc" {
		t.Fatalf("linux: %q", got)
	}
}

func TestLocallyAdministered(t *testing.T) {
	if IsLocallyAdministered("a4:2b:b0:00:00:01") {
		t.Fatal("a4 is globally unique")
	}
	if !IsLocallyAdministered("da:2b:b0:00:00:01") {
		t.Fatal("da has the local bit set")
	}
}

func TestTitleAndVendor(t *testing.T) {
	title := ExtractTitle("<html><head><TITLE>\n  Archer C6  </TITLE></head>")
	if title != "Archer C6" {
		t.Fatalf("title %q", title)
	}
	if v := GuessVendor(title); v != "TP-Link" {
		t.Fatalf("vendor %q", v)
	}
	if v := GuessVendor("lighttpd", "Welcome"); v != "" {
		t.Fatalf("unexpected vendor %q", v)
	}
}

func TestIsPrivate(t *testing.T) {
	for ip, want := range map[string]bool{"192.168.1.1": true, "10.0.0.1": true, "172.16.5.1": true, "100.64.0.1": true, "8.8.8.8": false, "1.0.30.172": false, "bogus": false} {
		if got := IsPrivate(ip); got != want {
			t.Errorf("IsPrivate(%s) = %v", ip, got)
		}
	}
}
