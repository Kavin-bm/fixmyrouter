package checks

import (
	"context"
	"net"
	"strings"
	"time"
)

// Search targets: the port-forwarding service (IGD) is what matters; a plain
// root-device reply can just be a media server on the router.
const (
	stIGD  = "urn:schemas-upnp-org:device:InternetGatewayDevice:1"
	stRoot = "upnp:rootdevice"
)

func mSearch(st string) []byte {
	return []byte("M-SEARCH * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 1\r\n" +
		"ST: " + st + "\r\n\r\n")
}

// UPnP sends the standard SSDP discovery packet (the same one phones and TVs
// send) unicast to the gateway and to the SSDP multicast group, and only
// reads replies that come from the gateway.
func UPnP(ctx context.Context, ip string) Finding {
	f := Finding{ID: "upnp", Title: "UPnP port forwarding (IGD)"}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return errFinding(f, err)
	}
	defer conn.Close()
	gw := net.ParseIP(ip)
	for _, t := range []*net.UDPAddr{{IP: gw, Port: 1900}, {IP: net.IPv4(239, 255, 255, 250), Port: 1900}} {
		for _, st := range []string{stIGD, stRoot} {
			_, _ = conn.WriteToUDP(mSearch(st), t)
		}
	}
	deadline := time.Now().Add(2500 * time.Millisecond)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetReadDeadline(deadline)
	go func() { <-ctx.Done(); _ = conn.SetReadDeadline(time.Now()) }()
	var replies []string
	buf := make([]byte, 4096)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}
		if from.IP.Equal(gw) {
			replies = append(replies, string(buf[:n]))
		}
	}
	if ctx.Err() != nil {
		return errFinding(f, ctx.Err())
	}
	return upnpFinding(f, replies)
}

func upnpFinding(f Finding, replies []string) Finding {
	server := ""
	for _, r := range replies {
		if server == "" {
			server = ssdpHeader(r, "server")
		}
		if isIGD(r) {
			f.Status, f.Severity = StatusWarn, "medium"
			f.Detail = "Your router offers UPnP port forwarding. It lets any app or device on your network open ports to the internet without asking you, which malware abuses."
			if server != "" {
				f.Detail += " Server: " + server + "."
			}
			f.Fix = "If you don't need it (some game consoles and video calls do), turn off UPnP in the router admin page, usually under Advanced / NAT Forwarding / UPnP."
			return f
		}
	}
	f.Status, f.Severity = StatusPass, "info"
	if len(replies) > 0 {
		f.Detail = "The router answers UPnP discovery (often a media server) but does not advertise port forwarding."
	} else {
		f.Detail = "No UPnP/SSDP reply from the gateway within 2.5s."
	}
	return f
}

func isIGD(reply string) bool {
	s := strings.ToLower(ssdpHeader(reply, "st") + " " + ssdpHeader(reply, "usn"))
	return strings.Contains(s, "internetgatewaydevice") || strings.Contains(s, "wanipconnection") || strings.Contains(s, "wanpppconnection")
}

func ssdpHeader(msg, name string) string {
	for _, line := range strings.Split(msg, "\r\n") {
		k, v, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(k), name) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
