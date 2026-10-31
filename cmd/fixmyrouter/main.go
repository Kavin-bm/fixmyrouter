// Command fixmyrouter finds out why your home Wi-Fi is slow or unsafe and
// explains it in plain English. Read-only unless you say yes to a fix.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Kavin-bm/fixmyrouter/internal/discover"
	"github.com/Kavin-bm/fixmyrouter/internal/fix"
	"github.com/Kavin-bm/fixmyrouter/internal/mcp"
	"github.com/Kavin-bm/fixmyrouter/internal/report"
	"github.com/Kavin-bm/fixmyrouter/internal/run"
	"github.com/Kavin-bm/fixmyrouter/internal/store"
	"github.com/Kavin-bm/fixmyrouter/internal/ui"
	"github.com/Kavin-bm/fixmyrouter/profiles"
)

// version is set by the release build (-ldflags "-X main.version=...").
var version = "0.1.0-dev"

// Exit codes are stable so scripts and agents can rely on them.
const (
	exitOK       = 0 // ran, nothing to fix
	exitError    = 1 // could not run (no gateway, OS error, ...)
	exitUsage    = 2 // bad command or flag
	exitFindings = 3 // ran, and found something worth fixing
)

// errFindings signals a successful run that found issues.
var errFindings = errors.New("findings")

const usage = `fixmyrouter - find out why your Wi-Fi is slow or unsafe, in plain English.

Usage:
  fixmyrouter check [--json] [--load]   everything below in one go, with a 0-100 score
  fixmyrouter speed [--json] [--load]   Wi-Fi band/channel/signal, congestion, router latency
                                        and jitter, DNS speed; --load adds a bufferbloat test
                                        (downloads up to 200 MB)
  fixmyrouter audit [--json]            router security: admin page, Telnet, UPnP, Wi-Fi
                                        encryption (talks only to your router)
  fixmyrouter discover [--json] [--online]
                                        your default gateway and its vendor
  fixmyrouter fix [<finding-id>] [--yes] list fixes fixmyrouter can make, or apply one after a yes
  fixmyrouter undo [<change-id>] [--all] put back a change from its snapshot
  fixmyrouter history [--json]          past scores and every change made
  fixmyrouter ui [--port 7979] [--no-open]
                                        local web UI on 127.0.0.1 (nothing loads from the internet)
  fixmyrouter profiles validate [files or dirs...] | list
                                        check router profiles (for contributors)
  fixmyrouter mcp                       MCP server on stdio for Claude Code, Codex, Gemini CLI
  fixmyrouter version

Exit codes: 0 nothing to fix, 1 error, 2 usage, 3 found something to fix.

Router probes go only to your own default gateway. fixmyrouter never logs in
to your router, never tries passwords, and changes nothing without a yes.
State lives in ~/.fixmyrouter (snapshots and past results, never uploaded).
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(exitUsage)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "check", "speed", "audit":
		err = runCheck(ctx, cmd, args)
	case "discover":
		err = runDiscover(ctx, args)
	case "fix":
		err = runFix(args, os.Stdin)
	case "undo":
		err = runUndo(args)
	case "history":
		err = runHistory(args)
	case "ui":
		err = runUI(ctx, args)
	case "profiles":
		err = runProfiles(args)
	case "mcp":
		err = mcp.Serve(ctx, version, os.Stdin, os.Stdout)
	case "version", "--version", "-v":
		fmt.Println("fixmyrouter", version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(exitUsage)
	}
	switch {
	case errors.Is(err, errFindings):
		os.Exit(exitFindings)
	case err != nil:
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitError)
	}
}

// parse parses flags that may come before or after positional arguments.
func parse(fs *flag.FlagSet, args []string) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			pos = append(pos, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if f := fs.Lookup(name); f != nil && !strings.Contains(name, "=") && !isBool(f) && i+1 < len(args) {
			i++
			flags = append(flags, args[i]) // "--port 7878": the value is the next arg
		}
	}
	fs.Parse(flags) // ExitOnError: exits with code 2 on a bad flag
	return pos
}

func isBool(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

func runCheck(ctx context.Context, kind string, args []string) error {
	fs := flag.NewFlagSet(kind, flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print JSON (for coding agents and scripts)")
	load := fs.Bool("load", false, "bufferbloat test: ping 1.1.1.1 while downloading up to 200 MB for ~10 s")
	parse(fs, args)
	if kind == run.KindAudit && *load {
		return fmt.Errorf("--load only applies to speed and check")
	}
	if !*asJSON {
		fmt.Fprintln(os.Stderr, "Measuring (about 10 seconds)...")
	}
	a, err := run.Run(ctx, version, run.Options{Kind: kind, Load: *load})
	if err != nil {
		return err
	}
	prev := run.Previous(a)
	if err := run.Save(a); err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not save this result for before/after:", err)
	}
	if *asJSON {
		err = report.JSON(os.Stdout, a)
	} else {
		report.Text(os.Stdout, a, prev)
	}
	if err == nil && a.Summary["warn"]+a.Summary["fail"] > 0 {
		return errFindings
	}
	return err
}

func runDiscover(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	online := fs.Bool("online", false, "look up the vendor from the MAC prefix via api.macvendors.com (sends only the first 3 bytes)")
	parse(fs, args)
	g, err := discover.Run(ctx, *online)
	if err != nil {
		return err
	}
	if *asJSON {
		return report.JSON(os.Stdout, g)
	}
	report.GatewayText(os.Stdout, g)
	return nil
}

func runFix(args []string, stdin io.Reader) error {
	fs := flag.NewFlagSet("fix", flag.ExitOnError)
	yes := fs.Bool("yes", false, "apply without asking (only after the person agreed)")
	pos := parse(fs, args)
	if len(pos) == 0 {
		runs, err := run.History()
		if err != nil {
			return err
		}
		if len(runs) == 0 {
			fmt.Println("No results yet. Run `fixmyrouter check` first.")
			return nil
		}
		last := runs[len(runs)-1]
		n := 0
		for _, f := range last.Findings {
			if f.AutoFix != nil {
				n++
				fmt.Printf("fixmyrouter fix %-16s %s\n", f.ID, f.AutoFix.Summary)
			}
		}
		if n == 0 {
			fmt.Println("Nothing in your last result can be fixed automatically. Each finding's \"How to fix\" has the manual steps.")
		}
		return nil
	}
	f, from := run.FindAutoFix(pos[0])
	if f == nil {
		return fmt.Errorf("no automatic fix %q in your saved results; run `fixmyrouter check` and then `fixmyrouter fix` to list them", pos[0])
	}
	_, iface, err := discover.DefaultGateway()
	if err != nil {
		return err
	}
	c, err := fix.Plan(f.AutoFix, iface)
	if err != nil {
		return err
	}
	before := "automatic (from your router)"
	if len(c.Before) > 0 {
		before = strings.Join(c.Before, ", ")
	}
	fmt.Printf("Finding:  %s (from your %s result)\n", f.Title, from.GeneratedAt.Local().Format("Jan 2 15:04"))
	fmt.Printf("Change:   %s\n", f.AutoFix.Summary)
	fmt.Printf("Network:  %s\nBefore:   %s\nAfter:    %s\n", c.Service, before, strings.Join(c.After, ", "))
	fmt.Printf("Snapshot: saved to %s before anything changes; `fixmyrouter undo` puts it back.\n", store.Dir())
	if !*yes {
		fmt.Print("\nApply this change? Type yes to continue: ")
		line, _ := bufio.NewReader(stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(line)); a != "yes" && a != "y" {
			fmt.Println("Nothing changed.")
			return nil
		}
	}
	if err := fix.Apply(c); err != nil {
		return fmt.Errorf("change %d failed: %w (run `fixmyrouter undo %d` to be sure the old setting is back)", c.ID, err, c.ID)
	}
	fmt.Printf("Done (change %d). Run `fixmyrouter speed` to compare, or `fixmyrouter undo` to revert.\n", c.ID)
	return nil
}

func runUndo(args []string) error {
	fs := flag.NewFlagSet("undo", flag.ExitOnError)
	all := fs.Bool("all", false, "undo every change, newest first")
	pos := parse(fs, args)
	id := 0
	if len(pos) > 0 {
		var err error
		if id, err = strconv.Atoi(pos[0]); err != nil {
			return fmt.Errorf("change id must be a number, see `fixmyrouter history`")
		}
	}
	for n := 0; ; n++ {
		c, err := fix.Undo(id)
		if err != nil {
			if n > 0 {
				return nil
			}
			return err
		}
		restored := "automatic (from your router)"
		if len(c.Before) > 0 {
			restored = strings.Join(c.Before, ", ")
		}
		fmt.Printf("Undid change %d: DNS on %s is back to %s.\n", c.ID, c.Service, restored)
		if !*all {
			return nil
		}
	}
}

func runHistory(args []string) error {
	fs := flag.NewFlagSet("history", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	parse(fs, args)
	runs, err := run.History()
	if err != nil {
		return err
	}
	changes, err := fix.History()
	if err != nil {
		return err
	}
	if *asJSON {
		type score struct {
			Kind  string `json:"kind"`
			At    string `json:"generated_at"`
			Score int    `json:"score"`
		}
		var scores []score
		for _, r := range runs {
			scores = append(scores, score{r.Kind, r.GeneratedAt.Format("2006-01-02T15:04:05Z"), r.Score})
		}
		return report.JSON(os.Stdout, map[string]any{"scores": scores, "changes": changes})
	}
	fmt.Println("Scores:")
	for _, r := range runs {
		fmt.Printf("  %s  %-5s  %3d/100\n", r.GeneratedAt.Local().Format("2006-01-02 15:04"), r.Kind, r.Score)
	}
	if len(runs) == 0 {
		fmt.Println("  none yet")
	}
	fmt.Println("Changes:")
	for _, c := range changes {
		fmt.Printf("  #%d  %s  %-7s  %s\n", c.ID, c.AppliedAt.Local().Format("2006-01-02 15:04"), c.Status, c.Summary)
	}
	if len(changes) == 0 {
		fmt.Println("  none (fixmyrouter has never changed anything)")
	}
	return nil
}

func runUI(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ui", flag.ExitOnError)
	port := fs.Int("port", 7979, "port on 127.0.0.1 (a free one is picked if it's taken)")
	noOpen := fs.Bool("no-open", false, "don't open the browser")
	parse(fs, args)
	return ui.ListenAndServe(ctx, version, *port, func(url string) {
		fmt.Printf("fixmyrouter UI: %s (only reachable from this computer; Ctrl-C to stop)\n", url)
		if !*noOpen {
			openBrowser(url)
		}
	})
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func runProfiles(args []string) error {
	if len(args) == 0 || (args[0] != "validate" && args[0] != "list") {
		return fmt.Errorf("usage: fixmyrouter profiles validate [files or dirs...] | list")
	}
	if args[0] == "list" {
		all, err := profiles.All()
		for _, p := range all {
			v := "unverified"
			if p.Verified {
				v = "verified"
			}
			fmt.Printf("%-40s %s %s %s (%s)\n", p.File, p.Vendor, p.Model, p.HardwareVersion, v)
		}
		return err
	}
	if len(args) == 1 {
		all, err := profiles.All()
		if err != nil {
			return err
		}
		fmt.Printf("%d built-in profile(s) valid.\n", len(all))
		return nil
	}
	bad := 0
	for _, root := range args[1:] {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || (filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml") {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if _, err := profiles.Parse(path, b); err != nil {
				bad++
				fmt.Println("✗", err)
			} else {
				fmt.Println("✓", path)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if bad > 0 {
		return fmt.Errorf("%d profile(s) invalid", bad)
	}
	return nil
}
