package speed

import (
	"bufio"
	"context"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
)

// DNSResult is the measured lookup time for one resolver.
type DNSResult struct {
	Name     string  `json:"name"`
	Server   string  `json:"server"`
	MedianMs float64 `json:"median_ms"`
	Failures int     `json:"failures"`
	Queries  int     `json:"queries"`
	System   bool    `json:"system"`
}

// PublicResolvers compared against your current DNS.
var PublicResolvers = []struct{ Name, IP string }{
	{"Cloudflare", "1.1.1.1"},
	{"Google", "8.8.8.8"},
	{"Quad9", "9.9.9.9"},
}

// Popular names: answers are usually cached by every resolver, so this
// measures the round trip you pay on most page loads.
var testDomains = []string{"google.com", "youtube.com", "wikipedia.org", "github.com", "amazon.in"}

// SystemResolvers returns the DNS servers this machine is using.
func SystemResolvers() []string {
	var out []string
	if runtime.GOOS == "darwin" {
		if b, err := exec.Command("scutil", "--dns").Output(); err == nil {
			out = ParseScutilDNS(string(b))
		}
	}
	if len(out) == 0 {
		if b, err := os.ReadFile("/etc/resolv.conf"); err == nil {
			out = ParseResolvConf(string(b))
		}
	}
	return out
}

// ParseScutilDNS returns unique nameservers from `scutil --dns`, in order.
func ParseScutilDNS(s string) []string {
	var out []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), ":")
		if ok && strings.HasPrefix(strings.TrimSpace(k), "nameserver[") {
			v = strings.TrimSpace(v)
			if !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	return out
}

// ParseResolvConf returns nameserver lines from resolv.conf.
func ParseResolvConf(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "nameserver" {
			out = append(out, f[1])
		}
	}
	return out
}

// MeasureDNS times rounds × testDomains lookups against server.
func MeasureDNS(ctx context.Context, name, server string, rounds int) DNSResult {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 2 * time.Second}
			return d.DialContext(ctx, "udp", net.JoinHostPort(server, "53"))
		},
	}
	res := DNSResult{Name: name, Server: server}
	var times []float64
	for i := 0; i < rounds; i++ {
		for _, d := range testDomains {
			res.Queries++
			qctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			start := time.Now()
			_, err := r.LookupIPAddr(qctx, d)
			el := time.Since(start)
			cancel()
			if err != nil {
				res.Failures++
				if res.Failures >= 3 && len(times) == 0 {
					res.Queries = res.Failures // unreachable: stop early
					return res
				}
				continue
			}
			times = append(times, float64(el.Microseconds())/1000)
		}
	}
	sort.Float64s(times)
	res.MedianMs = round1(median(times))
	return res
}
