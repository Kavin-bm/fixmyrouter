package speed

// Congestion summarises how crowded the current channel is and which
// channel would be quieter.
type Congestion struct {
	Seen        int    `json:"networks_seen"`
	OnMyChannel int    `json:"overlapping_my_channel"`
	BestChannel int    `json:"quietest_channel"`
	BestCount   int    `json:"overlapping_quietest_channel"`
	Band        string `json:"band"`
	Note        string `json:"note,omitempty"`
}

// 2.4 GHz channels overlap when they are less than 5 apart.
func overlaps24(a, b int) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= 4
}

// block80 returns the 80 MHz block a 5 GHz channel belongs to.
func block80(ch int) int {
	switch {
	case ch >= 36 && ch <= 48:
		return 36
	case ch >= 52 && ch <= 64:
		return 52
	case ch >= 100 && ch <= 112:
		return 100
	case ch >= 116 && ch <= 128:
		return 116
	case ch >= 132 && ch <= 144:
		return 132
	case ch >= 149 && ch <= 161:
		return 149
	}
	return ch
}

// Analyze counts neighbours that share airtime with the current channel and
// finds the quietest standard alternative (1/6/11 on 2.4 GHz; the non-DFS
// blocks 36-48 and 149-161 on 5 GHz).
func Analyze(w WiFi) (Congestion, bool) {
	if w.Channel == 0 || len(w.NearbyChannels) == 0 || w.Band == "6GHz" {
		return Congestion{}, false
	}
	c := Congestion{Band: w.Band}
	var candidates []int
	var same func(a, b int) bool
	switch w.Band {
	case "2.4GHz":
		candidates, same = []int{1, 6, 11}, overlaps24
	case "5GHz":
		candidates = []int{36, 149}
		same = func(a, b int) bool { return block80(a) == block80(b) }
		c.Note = "Counts 80 MHz blocks. DFS channels (52-144) can also be quiet but may pause when radar is detected."
	default:
		return Congestion{}, false
	}
	count := func(ch int) int {
		n := 0
		for _, o := range w.NearbyChannels {
			if o.Band == w.Band && same(o.Channel, ch) {
				n++
			}
		}
		return n
	}
	for _, o := range w.NearbyChannels {
		if o.Band == w.Band {
			c.Seen++
		}
	}
	c.OnMyChannel = count(w.Channel)
	c.BestChannel, c.BestCount = candidates[0], count(candidates[0])
	for _, ch := range candidates[1:] {
		if n := count(ch); n < c.BestCount {
			c.BestChannel, c.BestCount = ch, n
		}
	}
	return c, true
}
