package speed

import (
	"context"
	"math"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// PingStats summarises ICMP round-trip times from the system `ping`.
type PingStats struct {
	Target   string  `json:"target"`
	Sent     int     `json:"sent"`
	Received int     `json:"received"`
	LossPct  float64 `json:"loss_pct"`
	MinMs    float64 `json:"min_ms"`
	MedianMs float64 `json:"median_ms"`
	AvgMs    float64 `json:"avg_ms"`
	MaxMs    float64 `json:"max_ms"`
	JitterMs float64 `json:"jitter_ms"` // mean absolute difference between consecutive replies
}

var timeRe = regexp.MustCompile(`time[=<]([0-9.]+) ?ms`)

// Ping runs the system ping (no root needed) count times, 0.2 s apart.
func Ping(ctx context.Context, target string, count int) (PingStats, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(count)*300*time.Millisecond+5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ping", "-c", strconv.Itoa(count), "-i", "0.2", target).Output()
	if len(out) == 0 && err != nil {
		return PingStats{Target: target, Sent: count, LossPct: 100}, err
	}
	return ParsePing(string(out), target, count), nil
}

// ParsePing builds stats from ping output lines.
func ParsePing(out, target string, sent int) PingStats {
	var rtts []float64
	for _, m := range timeRe.FindAllStringSubmatch(out, -1) {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			rtts = append(rtts, v)
		}
	}
	return Stats(rtts, target, sent)
}

// Stats computes summary numbers from RTT samples in milliseconds.
func Stats(rtts []float64, target string, sent int) PingStats {
	s := PingStats{Target: target, Sent: sent, Received: len(rtts)}
	if sent > 0 {
		s.LossPct = round1(float64(sent-len(rtts)) / float64(sent) * 100)
	}
	if len(rtts) == 0 {
		return s
	}
	var sum, jit float64
	for i, v := range rtts {
		sum += v
		if i > 0 {
			jit += math.Abs(v - rtts[i-1])
		}
	}
	sorted := append([]float64(nil), rtts...)
	sort.Float64s(sorted)
	s.MinMs, s.MaxMs = round1(sorted[0]), round1(sorted[len(sorted)-1])
	s.MedianMs = round1(median(sorted))
	s.AvgMs = round1(sum / float64(len(rtts)))
	if len(rtts) > 1 {
		s.JitterMs = round1(jit / float64(len(rtts)-1))
	}
	return s
}

func median(sorted []float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
