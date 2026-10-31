package diagnose

import (
	"fmt"
	"math"
	"strings"
)

// MetricDelta is one measurement before and after a fix.
type MetricDelta struct {
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Unit        string  `json:"unit"`
	Before      float64 `json:"before"`
	After       float64 `json:"after"`
	Change      float64 `json:"change"`     // after - before
	ChangePct   float64 `json:"change_pct"` // relative to before
	Better      bool    `json:"better"`     // significant and in the good direction
	Worse       bool    `json:"worse"`      // significant and in the bad direction
	Target      bool    `json:"target"`     // the metric this fix was meant to improve
	Explanation string  `json:"explanation"`
}

// Delta compares two diagnoses.
type Delta struct {
	Metrics  []MetricDelta `json:"metrics"`
	Target   string        `json:"target,omitempty"`
	Improved bool          `json:"improved"`
	Verdict  string        `json:"verdict"`
}

type metricDef struct {
	name, unit    string
	higherBetter  bool
	minAbs, minPc float64 // a change counts only if both thresholds are met (measurement noise)
}

var metricDefs = map[string]metricDef{
	MetricDNS:          {"DNS lookup", "ms", false, 3, 10},
	MetricGateway:      {"Ping to router", "ms", false, 2, 15},
	MetricGatewayJit:   {"Router jitter", "ms", false, 2, 20},
	MetricGatewayLossP: {"Router packet loss", "%", false, 1, 0},
	MetricWAN:          {"Ping to internet", "ms", false, 3, 10},
	MetricWANLoss:      {"Internet packet loss", "%", false, 1, 0},
	MetricLoaded:       {"Latency under load", "ms", false, 10, 15},
	MetricDownload:     {"Download speed", "Mbps", true, 2, 10},
	MetricSignal:       {"Wi-Fi signal", "dBm", true, 3, 0},
	MetricNeighbours:   {"Neighbours on your channel", "", false, 1, 0},
}

var metricOrder = []string{MetricDNS, MetricLoaded, MetricDownload, MetricWAN, MetricWANLoss, MetricGateway, MetricGatewayJit, MetricGatewayLossP, MetricSignal, MetricNeighbours}

// Values extracts comparable numbers from a diagnosis.
func Values(d *Diagnosis) map[string]float64 {
	v := map[string]float64{}
	m := d.Measurements
	if r := m.Local; r != nil {
		for _, x := range r.DNS {
			if x.System && x.Queries > 0 && x.Failures < x.Queries {
				v[MetricDNS] = x.MedianMs
			}
		}
		if g := r.Gateway; g != nil && g.Received > 0 {
			v[MetricGateway], v[MetricGatewayJit], v[MetricGatewayLossP] = g.MedianMs, g.JitterMs, g.LossPct
		}
		if b := r.Bufferbloat; b != nil {
			v[MetricLoaded] = b.Loaded.MedianMs
		}
		if w := r.WiFi; w != nil && w.SignalDBm != 0 {
			v[MetricSignal] = float64(w.SignalDBm)
		}
		if c := r.Congestion; c != nil {
			v[MetricNeighbours] = float64(c.OnMyChannel)
		}
	}
	if w := BestWAN(m.WAN.Pings); w != nil {
		v[MetricWAN], v[MetricWANLoss] = w.MedianMs, w.LossPct
	}
	if mbps := downloadMbps(m); mbps > 0 {
		v[MetricDownload] = mbps
	}
	return v
}

// Compare reports what changed between before and after. target is the
// metric the fix was meant to improve ("" for any). It only says "improved"
// when the numbers show a change bigger than normal measurement noise.
func Compare(before, after *Diagnosis, target string) Delta {
	b, a := Values(before), Values(after)
	dl := Delta{Target: target}
	for _, k := range metricOrder {
		bv, okb := b[k]
		av, oka := a[k]
		if !okb || !oka {
			continue
		}
		def := metricDefs[k]
		md := MetricDelta{Key: k, Name: def.name, Unit: def.unit, Before: bv, After: av, Change: round1(av - bv), Target: k == target}
		if bv != 0 {
			md.ChangePct = round1(100 * (av - bv) / math.Abs(bv))
		}
		significant := math.Abs(av-bv) >= def.minAbs && (def.minPc == 0 || bv == 0 || math.Abs(md.ChangePct) >= def.minPc)
		good := (av > bv) == def.higherBetter
		md.Better, md.Worse = significant && good, significant && !good
		md.Explanation = describe(md, def)
		dl.Metrics = append(dl.Metrics, md)
	}
	var better, worse []string
	targetSeen, targetBetter, targetWorse := false, false, false
	for _, md := range dl.Metrics {
		if md.Target {
			targetSeen, targetBetter, targetWorse = true, md.Better, md.Worse
		}
		if md.Better {
			better = append(better, md.Explanation)
		}
		if md.Worse {
			worse = append(worse, md.Explanation)
		}
	}
	switch {
	case target != "" && !targetSeen:
		dl.Verdict = fmt.Sprintf("Can't tell: %s was not measured both times, so we make no claim.", strings.ToLower(metricDefs[target].name))
	case target != "" && targetBetter:
		dl.Improved = true
		dl.Verdict = "Measured improvement: " + strings.Join(better, "; ") + "."
	case target != "" && targetWorse:
		dl.Verdict = "It got worse: " + strings.Join(worse, "; ") + ". Consider undoing the change."
	case target == "" && len(better) > 0 && len(worse) == 0:
		dl.Improved = true
		dl.Verdict = "Measured improvement: " + strings.Join(better, "; ") + "."
	default:
		dl.Verdict = "No measurable gain (changes were within normal measurement noise). We won't claim it helped."
		if len(worse) > 0 {
			dl.Verdict = "No clear gain, and some numbers got worse: " + strings.Join(worse, "; ") + "."
		}
	}
	if dl.Improved && len(worse) > 0 {
		dl.Verdict += " But: " + strings.Join(worse, "; ") + "."
	}
	return dl
}

func describe(md MetricDelta, def metricDef) string {
	unit := def.unit
	if unit != "" && unit != "%" {
		unit = " " + unit
	}
	s := fmt.Sprintf("%s %s%s -> %s%s", def.name, fmtNum(md.Before), unit, fmtNum(md.After), unit)
	if md.Before != 0 && def.minPc > 0 {
		s += fmt.Sprintf(" (%+.0f%%)", md.ChangePct)
	}
	return s
}

func fmtNum(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
