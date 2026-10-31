package mcp

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Kavin-bm/fixmyrouter/internal/diagnose"
	"github.com/Kavin-bm/fixmyrouter/internal/isp"
	"github.com/Kavin-bm/fixmyrouter/internal/store"
)

// Prober and Sleep are swapped in tests.
var (
	Prober diagnose.Prober = diagnose.Real{}
	Sleep  isp.Sleep       = isp.RealSleep
	Settle                 = 3 * time.Second
)

// MaxReportDuration bounds isp_report so a tool call can't block for hours.
const MaxReportDuration = 15 * time.Minute

func boolArg(a map[string]any, k string) bool { v, _ := a[k].(bool); return v }
func numArg(a map[string]any, k string) float64 {
	v, _ := a[k].(float64)
	return v
}

func solveTools(version string) []tool {
	b := func(desc string) map[string]any { return map[string]any{"type": "boolean", "description": desc} }
	n := func(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }
	return []tool{
		{
			Name: "diagnose",
			Description: "Find the main reason the internet is slow: Wi-Fi, bufferbloat (lag under load), DNS, the router, or the ISP, with the numbers behind it and fixes ordered " +
				"auto (fixmyrouter changes this computer), router (needs the person's router password in a terminal), guided (click-by-click) and advice. " +
				"Read-only. load and throughput download data (up to 200 MB / throughput_mb): ask the person first. It cannot make the ISP faster.",
			InputSchema: schema(map[string]any{
				"load":          b("lag-under-load test, downloads up to 200 MB (ask first)"),
				"throughput":    b("download speed test from speed.cloudflare.com, capped (ask first)"),
				"throughput_mb": n("cap for the download test in MB (default 25, max 200)"),
				"plan_mbps":     n("the person's plan download speed in Mbps, if known"),
			}),
			call: func(ctx context.Context, a map[string]any) (any, error) {
				opt := diagnose.Options{Load: boolArg(a, "load"), Throughput: boolArg(a, "throughput"), ThroughputMB: int(numArg(a, "throughput_mb")), PlanMbps: numArg(a, "plan_mbps")}
				d, err := diagnose.Run(ctx, Prober, version, opt)
				if err != nil {
					return nil, err
				}
				_ = diagnose.Save(d)
				return d, nil
			},
		},
		{
			Name: "measure_fix",
			Description: "Apply or confirm one fix from the latest diagnose result, measure again with the same tests, and return before/after numbers with an honest verdict (it only says improved when the numbers show it). " +
				"For an auto fix: first show the person the fix's title and steps, get an explicit yes, then set user_confirmed=true; it saves a snapshot (undo reverts it). " +
				"For guided/advice fixes: give the person the steps, wait until they say they are done, then set user_confirmed=true. Router fixes are refused here: the person runs them in a terminal with their own password.",
			InputSchema: schema(map[string]any{
				"fix_id":         map[string]any{"type": "string", "description": "action id from diagnose, e.g. set-dns"},
				"user_confirmed": b("true only if the person said yes (auto) or said they finished the steps (guided/advice)"),
			}, "fix_id", "user_confirmed"),
			call: func(ctx context.Context, a map[string]any) (any, error) {
				id, _ := a["fix_id"].(string)
				return diagnose.MeasureFix(ctx, Prober, version, diagnose.Latest(), id, boolArg(a, "user_confirmed"), Settle)
			},
		},
		{
			Name: "isp_report",
			Description: fmt.Sprintf("Collect timestamped evidence for the internet provider: samples of router and internet latency/loss, DNS and download speed, written as Markdown and JSON in %s. "+
				"Blocks for (samples-1) x interval_seconds, at most %s; for longer runs tell the person to use `fixmyrouter report --isp`. "+
				"Unless no_download, it downloads throughput_mb (default 25) per sample: tell the person the total and set user_confirmed=true only after they agree.", filepath.Join(store.Dir(), "reports"), MaxReportDuration),
			InputSchema: schema(map[string]any{
				"samples":          n("number of samples (1-10)"),
				"interval_seconds": n("seconds between samples (min 10)"),
				"plan_mbps":        n("plan download speed in Mbps, if known"),
				"throughput_mb":    n("download cap per sample in MB (default 25)"),
				"no_download":      b("pings and DNS only, almost no data"),
				"user_confirmed":   b("true only after the person agreed to the data use"),
			}, "samples"),
			call: func(ctx context.Context, a map[string]any) (any, error) {
				o := isp.Options{Samples: int(numArg(a, "samples")), Interval: time.Duration(numArg(a, "interval_seconds")) * time.Second,
					PlanMbps: numArg(a, "plan_mbps"), ThroughputMB: int(numArg(a, "throughput_mb")), NoDownload: boolArg(a, "no_download")}
				if o.Samples > 10 {
					return nil, fmt.Errorf("at most 10 samples here; use `fixmyrouter report --isp` for more")
				}
				if err := o.Validate(); err != nil {
					return nil, err
				}
				if d := time.Duration(o.Samples-1) * o.Interval; d > MaxReportDuration {
					return nil, fmt.Errorf("that would take %s; at most %s here, use `fixmyrouter report --isp` for longer runs", d, MaxReportDuration)
				}
				if !o.NoDownload && !boolArg(a, "user_confirmed") {
					return nil, fmt.Errorf("refused: this downloads up to %d MB; tell the person, then call again with user_confirmed=true (or no_download=true)", o.DataBudgetMB())
				}
				r, err := isp.Collect(ctx, Prober, version, o, Sleep, nil)
				if err != nil {
					return nil, err
				}
				md, js, err := isp.Write(filepath.Join(store.Dir(), "reports"), r)
				return map[string]any{"markdown_file": md, "json_file": js, "conclusion": r.Conclusion, "summary": r.Summary}, err
			},
		},
	}
}
