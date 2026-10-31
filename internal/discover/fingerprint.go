package discover

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// vendorHints maps lowercase keywords seen in the admin page title or HTTP
// headers to a vendor name. Add more via pull request.
var vendorHints = []struct{ key, vendor string }{
	{"tp-link", "TP-Link"}, {"tplink", "TP-Link"}, {"archer", "TP-Link"},
	{"netgear", "Netgear"}, {"asus", "ASUS"}, {"d-link", "D-Link"}, {"dlink", "D-Link"},
	{"linksys", "Linksys"}, {"huawei", "Huawei"}, {"zte", "ZTE"}, {"tenda", "Tenda"},
	{"mikrotik", "MikroTik"}, {"routeros", "MikroTik"}, {"ubiquiti", "Ubiquiti"}, {"unifi", "Ubiquiti"},
	{"fritz", "AVM FRITZ!Box"}, {"jiofiber", "JioFiber"}, {"jio", "Jio"}, {"airtel", "Airtel"},
	{"syrotech", "Syrotech"}, {"netlink", "Netlink"}, {"nokia", "Nokia"}, {"sagemcom", "Sagemcom"},
	{"openwrt", "OpenWrt"}, {"luci", "OpenWrt"}, {"xiaomi", "Xiaomi"}, {"mi router", "Xiaomi"},
	{"mercusys", "Mercusys"}, {"cisco", "Cisco"}, {"technicolor", "Technicolor"}, {"arris", "ARRIS"},
	{"eero", "eero"}, {"google wifi", "Google"}, {"nest wifi", "Google"}, {"pfsense", "pfSense"},
}

var titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// ExtractTitle returns the trimmed contents of the first <title> tag.
func ExtractTitle(html string) string {
	m := titleRe.FindStringSubmatch(html)
	if m == nil {
		return ""
	}
	return strings.Join(strings.Fields(m[1]), " ")
}

// GuessVendor matches known keywords in free text (title, Server header, realm).
func GuessVendor(texts ...string) string {
	all := strings.ToLower(strings.Join(texts, " "))
	for _, h := range vendorHints {
		if strings.Contains(all, h.key) {
			return h.vendor
		}
	}
	return ""
}

// Fingerprint does one plain GET of http://<ip>/ (no redirects followed, no
// credentials sent) and records the Server header and page title.
func Fingerprint(ctx context.Context, g *Gateway) error {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s/", g.IP), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "fixmyrouter (read-only audit)")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	g.HTTPStatus = resp.StatusCode
	g.HTTPServer = resp.Header.Get("Server")
	g.HTTPTitle = ExtractTitle(string(body))
	g.HTTPRedirectLocation = resp.Header.Get("Location")
	if g.Vendor == "" {
		if v := GuessVendor(g.HTTPTitle, g.HTTPServer, resp.Header.Get("WWW-Authenticate")); v != "" {
			g.Vendor, g.VendorSource = v, "admin page"
		}
	}
	return nil
}

// LookupOUIOnline asks api.macvendors.com about the first 3 bytes of the MAC.
// Only used with the explicit --online flag; only the vendor prefix is sent.
func LookupOUIOnline(ctx context.Context, mac string) (string, error) {
	if len(mac) < 8 {
		return "", fmt.Errorf("invalid MAC")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.macvendors.com/"+mac[:8], nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vendor lookup: HTTP %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return strings.TrimSpace(string(b)), nil
}

// Run performs full discovery: gateway, admin-page fingerprint, then MAC
// from the ARP cache (the fingerprint request usually populates it).
func Run(ctx context.Context, online bool) (*Gateway, error) {
	ip, iface, err := DefaultGateway()
	if err != nil {
		return nil, err
	}
	g := &Gateway{IP: ip, Interface: iface}
	_ = Fingerprint(ctx, g) // no web UI on port 80 is fine
	g.MAC = LookupMAC(ip)
	if g.MAC != "" {
		g.MACLocallyAdmin = IsLocallyAdministered(g.MAC)
		if online && !g.MACLocallyAdmin {
			if v, err := LookupOUIOnline(ctx, g.MAC); err == nil && v != "" {
				g.Vendor, g.VendorSource = v, "MAC OUI (api.macvendors.com)"
			}
		}
	}
	return g, nil
}
