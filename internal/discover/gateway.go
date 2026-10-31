// Package discover finds the default gateway (your router) and tries to
// identify its vendor. Everything here is read-only.
package discover

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// Gateway describes the router this machine uses as its default route.
type Gateway struct {
	IP                   string `json:"ip"`
	Interface            string `json:"interface,omitempty"`
	MAC                  string `json:"mac,omitempty"`
	MACLocallyAdmin      bool   `json:"mac_locally_administered,omitempty"`
	Vendor               string `json:"vendor,omitempty"`
	VendorSource         string `json:"vendor_source,omitempty"`
	HTTPServer           string `json:"http_server,omitempty"`
	HTTPTitle            string `json:"http_title,omitempty"`
	HTTPStatus           int    `json:"http_status,omitempty"`
	HTTPRedirectLocation string `json:"http_redirect,omitempty"`
}

// ErrNoGateway is returned when no default route exists.
var ErrNoGateway = errors.New("no default gateway found (are you connected to a network?)")

// DefaultGateway returns the default gateway IP and the interface it is reached through.
func DefaultGateway() (ip, iface string, err error) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("route", "-n", "get", "default").Output()
		if err != nil {
			return "", "", fmt.Errorf("route -n get default: %w", err)
		}
		ip, iface = ParseRouteGet(string(out))
	case "linux":
		data, err := os.ReadFile("/proc/net/route")
		if err != nil {
			return "", "", err
		}
		ip, iface = ParseProcNetRoute(string(data))
	default:
		return "", "", fmt.Errorf("%s is not supported yet (macOS and Linux only for now)", runtime.GOOS)
	}
	if ip == "" {
		return "", "", ErrNoGateway
	}
	if !IsPrivate(ip) {
		return "", "", fmt.Errorf("default gateway %s is not a private LAN address; fixmyrouter only audits home routers on your local network", ip)
	}
	return ip, iface, nil
}

// IsPrivate reports whether ip is a LAN address (RFC 1918, CGNAT 100.64/10,
// link-local or ULA). fixmyrouter refuses to audit anything else.
func IsPrivate(ip string) bool {
	p := net.ParseIP(ip)
	if p == nil {
		return false
	}
	if p.IsPrivate() || p.IsLinkLocalUnicast() {
		return true
	}
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	return cgnat.Contains(p)
}

// ParseRouteGet parses macOS `route -n get default` output.
func ParseRouteGet(out string) (ip, iface string) {
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "gateway":
			ip = strings.TrimSpace(v)
		case "interface":
			iface = strings.TrimSpace(v)
		}
	}
	if net.ParseIP(ip) == nil {
		ip = "" // e.g. a VPN link with no IP gateway
	}
	return ip, iface
}

// ParseProcNetRoute parses Linux /proc/net/route and returns the default route
// with the lowest metric.
func ParseProcNetRoute(data string) (ip, iface string) {
	best := -1
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 7 || f[1] != "00000000" || f[0] == "Iface" {
			continue
		}
		raw, err := hex.DecodeString(f[2])
		if err != nil || len(raw) != 4 {
			continue
		}
		var metric int
		fmt.Sscanf(f[6], "%d", &metric)
		if best != -1 && metric >= best {
			continue
		}
		// The kernel prints the address as a little-endian hex word.
		gw := net.IPv4(raw[3], raw[2], raw[1], raw[0])
		if gw.IsUnspecified() {
			continue
		}
		best, ip, iface = metric, gw.String(), f[0]
	}
	return ip, iface
}

var arpDarwinRe = regexp.MustCompile(`\(([0-9.]+)\) at ([0-9a-fA-F:]+)`)

// LookupMAC reads the system ARP cache for ip. It sends no packets itself.
func LookupMAC(ip string) string {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("arp", "-n", ip).Output()
		if err != nil {
			return ""
		}
		return ParseArpDarwin(string(out), ip)
	case "linux":
		data, err := os.ReadFile("/proc/net/arp")
		if err != nil {
			return ""
		}
		return ParseProcNetArp(string(data), ip)
	}
	return ""
}

// ParseArpDarwin parses `arp -n <ip>` output on macOS (octets may lack leading zeros).
func ParseArpDarwin(out, ip string) string {
	for _, m := range arpDarwinRe.FindAllStringSubmatch(out, -1) {
		if m[1] == ip {
			return NormalizeMAC(m[2])
		}
	}
	return ""
}

// ParseProcNetArp parses Linux /proc/net/arp.
func ParseProcNetArp(data, ip string) string {
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) >= 4 && f[0] == ip && f[3] != "00:00:00:00:00:00" {
			return NormalizeMAC(f[3])
		}
	}
	return ""
}

// NormalizeMAC turns "a4:2b:b0:1:2:3" into "a4:2b:b0:01:02:03". Returns "" if invalid.
func NormalizeMAC(s string) string {
	parts := strings.Split(strings.ToLower(s), ":")
	if len(parts) != 6 {
		return ""
	}
	for i, p := range parts {
		if len(p) == 1 {
			parts[i] = "0" + p
		} else if len(p) != 2 {
			return ""
		}
	}
	return strings.Join(parts, ":")
}

// IsLocallyAdministered reports whether a MAC is randomized/locally assigned,
// in which case its OUI does not identify a vendor.
func IsLocallyAdministered(mac string) bool {
	if len(mac) < 2 {
		return false
	}
	b, err := hex.DecodeString(mac[:2])
	return err == nil && b[0]&0x02 != 0
}
