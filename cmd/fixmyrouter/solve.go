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
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Kavin-bm/fixmyrouter/internal/diagnose"
	"github.com/Kavin-bm/fixmyrouter/internal/discover"
	"github.com/Kavin-bm/fixmyrouter/internal/fix"
	"github.com/Kavin-bm/fixmyrouter/internal/report"
	"github.com/Kavin-bm/fixmyrouter/internal/routerfix"
	"github.com/Kavin-bm/fixmyrouter/internal/speed"
	"github.com/Kavin-bm/fixmyrouter/internal/store"
	"github.com/Kavin-bm/fixmyrouter/profiles"
)

// prober is swapped in tests.
var prober diagnose.Prober = diagnose.Real{}

func diagFlags(fs *flag.FlagSet) *diagnose.Options {
	o := &diagnose.Options{}
	fs.BoolVar(&o.Load, "load", false, "lag-under-load (bufferbloat) test: downloads up to 200 MB for ~10 s")
	fs.BoolVar(&o.Throughput, "throughput", false, "internet download speed test from speed.cloudflare.com (capped, see --throughput-mb)")
	fs.IntVar(&o.ThroughputMB, "throughput-mb", speed.DefaultThroughputMB, "data cap for --throughput, in MB (max 200)")
	fs.Float64Var(&o.PlanMbps, "plan", 0, "your plan's download speed in Mbps, to compare against")
	return o
}

func measure(ctx context.Context, opt diagnose.Options, quiet bool) (*diagnose.Diagnosis, error) {
	if !quiet {
		msg := "Measuring Wi-Fi, router, DNS and the internet path (about 10 seconds"
		if opt.Load {
			msg += ", +10 s lag-under-load test"
		}
		if opt.Throughput {
			msg += fmt.Sprintf(", +up to 12 s download test capped at %d MB", opt.ThroughputMB)
		}
		fmt.Fprintln(os.Stderr, msg+")...")
	}
	d, err := diagnose.Run(ctx, prober, version, opt)
	if err != nil {
		return nil, err
	}
	if err := diagnose.Save(d); err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not save this result:", err)
	}
	return d, nil
}

func runDiagnose(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("diagnose", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	opt := diagFlags(fs)
	parse(fs, args)
	d, err := measure(ctx, *opt, *asJSON)
	if err != nil {
		return err
	}
	if *asJSON {
		err = report.JSON(os.Stdout, d)
	} else {
		report.DiagnosisText(os.Stdout, d)
	}
	if err == nil && !d.Healthy {
		return errFindings
	}
	return err
}

// prompt asks a question on stdout and returns the trimmed answer.
func prompt(in *bufio.Reader, q string) string {
	fmt.Print(q)
	line, _ := in.ReadString('\n')
	return strings.TrimSpace(line)
}

func confirmed(in *bufio.Reader, q string) bool {
	a := strings.ToLower(prompt(in, q+" Type yes to continue: "))
	return a == "yes" || a == "y"
}

func runFixMeasure(ctx context.Context, args []string, stdin io.Reader) error {
	fs := flag.NewFlagSet("fix", flag.ExitOnError)
	fs.Bool("measure", true, "diagnose, fix, measure again and show the difference")
	yes := fs.Bool("yes", false, "apply without asking (only after the person agreed)")
	user := fs.String("user", "root", "router admin user, for router fixes")
	https := fs.Bool("https", false, "talk to the router over HTTPS, for router fixes")
	asJSON := fs.Bool("json", false, "print the result as JSON at the end")
	opt := diagFlags(fs)
	pos := parse(fs, args)
	in := bufio.NewReader(stdin)

	fmt.Println("Step 1/3: measuring before the fix.")
	before, err := measure(ctx, *opt, false)
	if err != nil {
		return err
	}
	var act *diagnose.Action
	if len(pos) > 0 {
		if act, _ = before.FindAction(pos[0]); act == nil {
			report.DiagnosisText(os.Stdout, before)
			return fmt.Errorf("fix %q doesn't apply to what we just measured; pick one of the ids above", pos[0])
		}
	} else if p := before.PrimaryBottleneck(); p != nil && len(p.Actions) > 0 {
		act = &p.Actions[0]
	}
	report.DiagnosisText(os.Stdout, before)
	if act == nil {
		fmt.Println("\nNothing to fix in what we measured, so nothing was changed.")
		return nil
	}
	if act.Metric == "" {
		fmt.Printf("\n%q has no before/after measurement: %s\n", act.ID, act.Command)
		return nil
	}
	if f := diagnose.MissingTest(act.Metric, *opt); f != "" {
		return fmt.Errorf("to measure %q before and after, run again with %s (it downloads data, so it needs your ok)", act.ID, f)
	}

	fmt.Printf("\nStep 2/3: %s [%s]\n", act.Title, act.Level)
	res := diagnose.Result{Action: *act, Before: before}
	settle := 3 * time.Second
	switch act.Level {
	case diagnose.LevelAuto:
		c, err := diagnose.PlanAuto(act)
		if err != nil {
			return err
		}
		was := "automatic (from your router)"
		if len(c.Before) > 0 {
			was = strings.Join(c.Before, ", ")
		}
		fmt.Printf("Change:   %s\nNetwork:  %s\nBefore:   %s\nAfter:    %s\n", act.AutoFix.Summary, c.Service, was, strings.Join(c.After, ", "))
		fmt.Printf("Snapshot: saved in %s first; `fixmyrouter undo` puts it back.\n", store.Dir())
		if !*yes && !confirmed(in, "\nApply this change?") {
			fmt.Println("Nothing changed.")
			return nil
		}
		if err := fix.Apply(c); err != nil {
			return fmt.Errorf("change %d failed: %w (run `fixmyrouter undo %d` to be sure the old setting is back)", c.ID, err, c.ID)
		}
		res.ChangeID, res.Undo = c.ID, fmt.Sprintf("fixmyrouter undo %d", c.ID)
	case diagnose.LevelRouter:
		c, err := routerChange(ctx, in, before.Measurements.Gateway, act.RouterSetting, act.RouterValue, *user, *https, *yes)
		if err != nil || c == nil {
			return err
		}
		res.ChangeID, res.Undo = c.ID, fmt.Sprintf("fixmyrouter router undo %d", c.ID)
		fmt.Println("Waiting 20 s for the router and Wi-Fi to settle...")
		settle = 20 * time.Second
	default:
		for i, s := range act.Steps {
			fmt.Printf("  %d. %s\n", i+1, s)
		}
		if a := prompt(in, "\nDo these steps now. Press Enter when done to measure again (or type q to stop): "); strings.EqualFold(a, "q") {
			fmt.Println("Stopped; nothing was measured after.")
			return nil
		}
		settle = 0
	}

	fmt.Println("\nStep 3/3: measuring again with the same tests.")
	after, dl, err := diagnose.Remeasure(ctx, prober, version, before, act.Metric, settle)
	if err != nil {
		return err
	}
	_ = diagnose.Save(after)
	res.After, res.Delta = after, dl
	if *asJSON {
		return report.JSON(os.Stdout, res)
	}
	report.DeltaText(os.Stdout, dl)
	if res.Undo != "" && !dl.Improved {
		fmt.Printf("To put the old setting back: %s\n", res.Undo)
	}
	return nil
}

// routerChange logs in once with the typed password, shows the exact change,
// asks, then applies it with a snapshot. It returns nil, nil if declined.
func routerChange(ctx context.Context, in *bufio.Reader, g *discover.Gateway, setting, value, user string, https, yes bool) (*routerfix.Change, error) {
	if g == nil {
		return nil, fmt.Errorf("no gateway found")
	}
	if diagnose.IsHotspot(g.IP) {
		return nil, fmt.Errorf("%s is a phone hotspot, not a router fixmyrouter can change", g.IP)
	}
	p := profiles.Find(g.HTTPTitle, g.HTTPServer)
	d, err := routerfix.DriverFor(p, setting)
	if err != nil {
		return nil, err
	}
	base, err := routerfix.BaseURL(g.IP, https)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Router:   %s %s at %s (profile %s)\n", p.Vendor, p.Model, base, p.File)
	if !p.Verified {
		fmt.Println("Note:     this router profile has not been verified on real hardware yet.")
	}
	pw, err := readPassword(in, fmt.Sprintf("Admin password for %q (sent once to %s, never stored): ", user, g.IP))
	if err != nil {
		return nil, err
	}
	s, err := d.Login(ctx, base, user, pw)
	if err != nil {
		return nil, err
	}
	c, err := routerfix.Plan(ctx, d, s, p, g.IP, setting, value)
	if err != nil {
		return nil, err
	}
	fmt.Printf("Change:   %s\n", c.Summary)
	for _, l := range routerfix.Describe(c.Edits) {
		fmt.Println("          " + l)
	}
	fmt.Printf("Snapshot: saved in %s first; `fixmyrouter router undo` puts it back.\n", store.Dir())
	if !yes && !confirmed(in, "\nApply this change on the router?") {
		fmt.Println("Nothing changed.")
		return nil, nil
	}
	if err := routerfix.Apply(ctx, s, c); err != nil {
		return c, fmt.Errorf("router change %d failed: %w (run `fixmyrouter router undo %d`)", c.ID, err, c.ID)
	}
	fmt.Printf("Done (router change %d).\n", c.ID)
	return c, nil
}

// readPassword reads a line with terminal echo off where possible.
func readPassword(in *bufio.Reader, q string) (string, error) {
	fmt.Print(q)
	echoOff := false
	if runtime.GOOS != "windows" {
		if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
			cmd := exec.Command("stty", "-echo")
			cmd.Stdin = os.Stdin
			echoOff = cmd.Run() == nil
		}
	}
	line, err := in.ReadString('\n')
	if echoOff {
		cmd := exec.Command("stty", "echo")
		cmd.Stdin = os.Stdin
		_ = cmd.Run()
		fmt.Println()
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		if err == nil {
			err = errors.New("no password typed")
		}
		return "", err
	}
	return line, nil
}

const routerUsage = `usage:
  fixmyrouter router apply <setting> <value> [--user root] [--https] [--yes]
  fixmyrouter router undo [<change-id>] [--user root] [--https]
  fixmyrouter router history
settings (only where your router's profile allows them): wifi_channel_2g, wifi_channel_5g, sqm_qos (value "DOWN[/UP]" Mbps)`

func runRouter(ctx context.Context, args []string, stdin io.Reader) error {
	fs := flag.NewFlagSet("router", flag.ExitOnError)
	user := fs.String("user", "root", "router admin user")
	https := fs.Bool("https", false, "talk to the router over HTTPS")
	yes := fs.Bool("yes", false, "apply without asking (only after the person agreed)")
	pos := parse(fs, args)
	in := bufio.NewReader(stdin)
	if len(pos) == 0 {
		return errors.New(routerUsage)
	}
	switch pos[0] {
	case "history":
		all, err := routerfix.History()
		for _, c := range all {
			fmt.Printf("#%d  %s  %-7s  %s  %s\n", c.ID, c.AppliedAt.Local().Format("2006-01-02 15:04"), c.Status, c.Router, c.Summary)
		}
		if err == nil && len(all) == 0 {
			fmt.Println("fixmyrouter has never changed a router setting.")
		}
		return err
	case "apply":
		if len(pos) != 3 {
			return errors.New(routerUsage)
		}
		g, err := discover.Run(ctx, false)
		if err != nil {
			return err
		}
		_, err = routerChange(ctx, in, g, pos[1], pos[2], *user, *https, *yes)
		return err
	case "undo":
		id := 0
		if len(pos) > 1 {
			var err error
			if id, err = strconv.Atoi(pos[1]); err != nil {
				return errors.New("change id must be a number, see `fixmyrouter router history`")
			}
		}
		c, err := routerfix.Pending(id)
		if err != nil {
			return err
		}
		g, err := discover.Run(ctx, false)
		if err != nil {
			return err
		}
		if g.IP != c.Router {
			return fmt.Errorf("change %d was made on %s, but your gateway is now %s; connect to that network first", c.ID, c.Router, g.IP)
		}
		p := profiles.Find(g.HTTPTitle, g.HTTPServer)
		d, err := routerfix.DriverFor(p, c.Setting)
		if err != nil {
			return err
		}
		base, err := routerfix.BaseURL(c.Router, *https)
		if err != nil {
			return err
		}
		fmt.Printf("Undo:     #%d %s on %s\n", c.ID, c.Summary, c.Router)
		pw, err := readPassword(in, fmt.Sprintf("Admin password for %q (sent once, never stored): ", *user))
		if err != nil {
			return err
		}
		s, err := d.Login(ctx, base, *user, pw)
		if err != nil {
			return err
		}
		if _, err := routerfix.Undo(ctx, s, c.ID); err != nil {
			return err
		}
		fmt.Printf("Undid router change %d.\n", c.ID)
		return nil
	}
	return errors.New(routerUsage)
}
