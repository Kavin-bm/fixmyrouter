package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Kavin-bm/fixmyrouter/internal/isp"
	"github.com/Kavin-bm/fixmyrouter/internal/speed"
)

// sleep is swapped in tests.
var sleep isp.Sleep = isp.RealSleep

func runReport(ctx context.Context, args []string, stdin io.Reader) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	ispFlag := fs.Bool("isp", false, "collect timestamped evidence for your internet provider")
	samples := fs.Int("samples", 6, "number of samples")
	interval := fs.Duration("interval", 10*time.Minute, "time between samples (min 10s)")
	out := fs.String("out", ".", "folder for the Markdown and JSON files")
	plan := fs.Float64("plan", 0, "your plan's download speed in Mbps")
	mb := fs.Int("throughput-mb", speed.DefaultThroughputMB, "download cap per sample, MB")
	noDL := fs.Bool("no-download", false, "skip the download test (pings and DNS only, almost no data)")
	yes := fs.Bool("yes", false, "start without asking about the data budget")
	parse(fs, args)
	if !*ispFlag {
		return errors.New("usage: fixmyrouter report --isp [--samples 6] [--interval 10m] [--plan Mbps] [--out dir] [--no-download]")
	}
	o := isp.Options{Samples: *samples, Interval: *interval, PlanMbps: *plan, ThroughputMB: *mb, NoDownload: *noDL}
	if err := o.Validate(); err != nil {
		return err
	}
	total := time.Duration(o.Samples-1) * o.Interval
	fmt.Printf("ISP report: %d sample(s), every %s (about %s in total).\n", o.Samples, o.Interval, total.Round(time.Minute))
	if o.NoDownload {
		fmt.Println("Data: pings and DNS only (well under 1 MB).")
	} else {
		fmt.Printf("Data: up to %d MB from speed.cloudflare.com (%d MB per sample). Mind metered or mobile connections.\n", o.DataBudgetMB(), o.ThroughputMB)
	}
	if !*yes && !confirmed(bufio.NewReader(stdin), "Start?") {
		fmt.Println("Not started.")
		return nil
	}
	r, err := isp.Collect(ctx, prober, version, o, sleep, func(i int, s isp.Sample) {
		line := fmt.Sprintf("[%s] sample %d/%d:", s.At.Local().Format("15:04:05"), i+1, o.Samples)
		if s.WAN != nil {
			line += fmt.Sprintf(" internet %.0f ms, %.0f%% loss", s.WAN.MedianMs, s.WAN.LossPct)
		}
		if s.DownloadMbps > 0 {
			line += fmt.Sprintf(", %.1f Mbps", s.DownloadMbps)
		}
		if len(s.Problems) > 0 {
			line += " - " + strings.Join(s.Problems, "; ")
		}
		fmt.Println(line)
	})
	if err != nil {
		return err
	}
	md, js, err := isp.Write(*out, r)
	if err != nil {
		return err
	}
	fmt.Printf("\n%s\n\nWrote %s\n  and %s\n", r.Conclusion, md, js)
	return nil
}
