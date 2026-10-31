// Command fixmyrouter audits your home Wi-Fi router, read-only by default.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/kavinbm/fixmyrouter/internal/checks"
	"github.com/kavinbm/fixmyrouter/internal/discover"
	"github.com/kavinbm/fixmyrouter/internal/report"
	"github.com/kavinbm/fixmyrouter/internal/speed"
)

const version = "0.1.0-dev"

const usage = `fixmyrouter - your Wi-Fi is slower than it should be. Find out why. Read-only.

Usage:
  fixmyrouter speed [--json] [--load]        Wi-Fi channel/signal, congestion, router ping, DNS speed,
                                             bufferbloat (--load downloads up to 200 MB), suggestions
  fixmyrouter discover [--json] [--online]   show your default gateway and vendor
  fixmyrouter audit [--json]                 read-only security checks on your own gateway
  fixmyrouter undo                           (coming soon) restore snapshots of any fix
  fixmyrouter mcp                            (coming soon) MCP server for coding agents
  fixmyrouter version

fixmyrouter only talks to your own default gateway. It never tries passwords
and never changes a setting.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "discover":
		err = runDiscover(ctx, args)
	case "speed":
		err = runSpeed(ctx, args)
	case "audit":
		err = runAudit(ctx, args)
	case "undo":
		fmt.Println("undo: coming soon. fixmyrouter has not changed anything on your router yet, so there is nothing to undo.")
	case "mcp":
		fmt.Println("mcp: coming soon. For now, agents can run `fixmyrouter audit --json` (see PROMPT.md).")
	case "version", "--version", "-v":
		fmt.Println("fixmyrouter", version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runDiscover(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	online := fs.Bool("online", false, "look up the vendor from the MAC prefix via api.macvendors.com (sends only the first 3 bytes)")
	fs.Parse(args)
	g, err := discover.Run(ctx, *online)
	if err != nil {
		return err
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(g)
	}
	report.GatewayText(os.Stdout, g)
	return nil
}

func runAudit(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print JSON (for coding agents)")
	fs.Parse(args)
	g, err := discover.Run(ctx, false)
	if err != nil {
		return err
	}
	a := &report.Audit{
		Tool: "fixmyrouter", Version: version, GeneratedAt: time.Now().UTC(), ReadOnly: true, Gateway: g,
		ComingSoon: []string{"wps", "dns-hijack", "wan-remote-admin", "firmware-age", "wifi-security-mode", "fix", "undo", "mcp"},
	}
	for _, c := range checks.All() {
		a.Findings = append(a.Findings, c.Run(ctx, g.IP))
	}
	a.Summary = report.Summarize(a.Findings)
	if *asJSON {
		return report.JSON(os.Stdout, a)
	}
	report.Text(os.Stdout, a)
	return nil
}

func runSpeed(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("speed", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print JSON (for coding agents)")
	load := fs.Bool("load", false, "also test bufferbloat: ping 1.1.1.1 while downloading up to 200 MB for ~10 s")
	fs.Parse(args)
	ip, _, err := discover.DefaultGateway()
	if err != nil {
		return err
	}
	if !*asJSON {
		fmt.Fprintln(os.Stderr, "Measuring (about 15 seconds)...")
	}
	r := speed.Run(ctx, ip, speed.Options{Load: *load})
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	}
	report.SpeedText(os.Stdout, r)
	return nil
}
