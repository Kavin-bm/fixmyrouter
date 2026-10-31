package checks

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// HTTPAdmin checks whether the router's admin page is served over plain HTTP,
// where the admin password crosses your Wi-Fi unencrypted.
func HTTPAdmin(ctx context.Context, ip string) Finding {
	return httpAdmin(ctx, "http://"+net.JoinHostPort(ip, "80")+"/", net.JoinHostPort(ip, "443"))
}

func httpAdmin(ctx context.Context, url, httpsAddr string) Finding {
	f := Finding{ID: "http-admin", Title: "Admin page encryption (HTTP vs HTTPS)"}
	rctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, url, nil)
	if err != nil {
		return errFinding(f, err)
	}
	req.Header.Set("User-Agent", "fixmyrouter (read-only audit)")
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return errFinding(f, ctx.Err())
		}
		f.Status, f.Severity = StatusPass, "info"
		f.Detail = "No web admin page answered on port 80."
		return f
	}
	resp.Body.Close()
	if loc := resp.Header.Get("Location"); resp.StatusCode >= 300 && resp.StatusCode < 400 && strings.HasPrefix(strings.ToLower(loc), "https://") {
		f.Status, f.Severity = StatusPass, "info"
		f.Detail = "Port 80 redirects to HTTPS (" + loc + ")."
		return f
	}
	f.Status, f.Severity = StatusWarn, "medium"
	f.Detail = fmt.Sprintf("The admin page answers on plain HTTP (status %d). Anyone on your Wi-Fi who can sniff traffic could read the admin password when you log in.", resp.StatusCode)
	if tcpOpen(ctx, httpsAddr) {
		f.Detail += " HTTPS (port 443) is also open."
		f.Fix = "Use https://<router-ip>/ to log in, and turn on 'HTTPS only' / 'redirect HTTP to HTTPS' in the router's admin settings if available."
	} else {
		f.Fix = "Only log in from a wired or trusted device, use a strong unique admin password, and check for a firmware update that adds HTTPS."
	}
	return f
}

// tcpOpen opens and immediately closes a TCP connection. Nothing is sent.
func tcpOpen(ctx context.Context, addr string) bool {
	d := net.Dialer{Timeout: dialTimeout}
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func errFinding(f Finding, err error) Finding {
	f.Status, f.Severity, f.Detail = StatusError, "info", err.Error()
	return f
}
