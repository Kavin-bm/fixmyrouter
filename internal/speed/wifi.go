package speed

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// WiFi is the current Wi-Fi link as reported by the host OS.
type WiFi struct {
	Interface      string   `json:"interface,omitempty"`
	Channel        int      `json:"channel,omitempty"`
	Band           string   `json:"band,omitempty"` // "2.4GHz", "5GHz", "6GHz"
	WidthMHz       int      `json:"width_mhz,omitempty"`
	SignalDBm      int      `json:"signal_dbm,omitempty"`
	NoiseDBm       int      `json:"noise_dbm,omitempty"`
	PHYMode        string   `json:"phy_mode,omitempty"`
	TxRateMbps     int      `json:"tx_rate_mbps,omitempty"`
	Supports5GHz   bool     `json:"supports_5ghz"`
	Source         string   `json:"source,omitempty"`
	NearbyChannels []Nearby `json:"nearby,omitempty"` // other networks the OS can see
}

// Nearby is one other network seen in the last OS scan (names are not kept).
type Nearby struct {
	Channel int    `json:"channel"`
	Band    string `json:"band"`
}

// BandForChannel guesses the band from a channel number (6 GHz is only known from frequency).
func BandForChannel(ch int) string {
	if ch >= 1 && ch <= 14 {
		return "2.4GHz"
	}
	if ch >= 32 && ch <= 177 {
		return "5GHz"
	}
	return ""
}

// ChannelFromFreq converts MHz to a channel and band.
func ChannelFromFreq(mhz int) (int, string) {
	switch {
	case mhz == 2484:
		return 14, "2.4GHz"
	case mhz >= 2412 && mhz <= 2472:
		return (mhz - 2407) / 5, "2.4GHz"
	case mhz >= 5160 && mhz <= 5885:
		return (mhz - 5000) / 5, "5GHz"
	case mhz >= 5955 && mhz <= 7115:
		return (mhz - 5950) / 5, "6GHz"
	}
	return 0, ""
}

// DetectWiFi reads the current Wi-Fi link. ok is false when not on Wi-Fi
// (wired, or the OS tools are missing).
func DetectWiFi() (WiFi, bool) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("system_profiler", "SPAirPortDataType", "-json").Output()
		if err != nil {
			return WiFi{}, false
		}
		return ParseSPAirPort(out)
	case "linux":
		return detectLinuxWiFi()
	}
	return WiFi{}, false
}

var chanRe = regexp.MustCompile(`^(\d+)\s*\((\d)GHz(?:,\s*(\d+)MHz)?\)`)

// parseMacChannel parses "149 (5GHz, 80MHz)" or "6 (2GHz, 20MHz)".
func parseMacChannel(s string) (ch int, band string, width int) {
	m := chanRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, "", 0
	}
	ch, _ = strconv.Atoi(m[1])
	switch m[2] {
	case "2":
		band = "2.4GHz"
	case "5":
		band = "5GHz"
	case "6":
		band = "6GHz"
	}
	width, _ = strconv.Atoi(m[3])
	return ch, band, width
}

// ParseSPAirPort parses `system_profiler SPAirPortDataType -json` (macOS).
func ParseSPAirPort(data []byte) (WiFi, bool) {
	var doc struct {
		Data []struct {
			Interfaces []struct {
				Name      string           `json:"_name"`
				Current   map[string]any   `json:"spairport_current_network_information"`
				Supported []string         `json:"spairport_supported_channels"`
				Others    []map[string]any `json:"spairport_airport_other_local_wireless_networks"`
			} `json:"spairport_airport_interfaces"`
		} `json:"SPAirPortDataType"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return WiFi{}, false
	}
	for _, d := range doc.Data {
		for _, ifc := range d.Interfaces {
			chs, _ := ifc.Current["spairport_network_channel"].(string)
			if chs == "" {
				continue
			}
			w := WiFi{Interface: ifc.Name, Source: "system_profiler"}
			w.Channel, w.Band, w.WidthMHz = parseMacChannel(chs)
			if sn, ok := ifc.Current["spairport_signal_noise"].(string); ok {
				// "-38 dBm / -93 dBm"
				parts := strings.Split(sn, "/")
				if len(parts) == 2 {
					w.SignalDBm, _ = strconv.Atoi(strings.Fields(parts[0])[0])
					w.NoiseDBm, _ = strconv.Atoi(strings.Fields(parts[1])[0])
				}
			}
			w.PHYMode, _ = ifc.Current["spairport_network_phymode"].(string)
			if r, ok := ifc.Current["spairport_network_rate"].(float64); ok {
				w.TxRateMbps = int(r)
			}
			for _, s := range ifc.Supported {
				if strings.Contains(s, "5GHz") {
					w.Supports5GHz = true
					break
				}
			}
			for _, o := range ifc.Others {
				if s, ok := o["spairport_network_channel"].(string); ok {
					if ch, band, _ := parseMacChannel(s); ch > 0 {
						w.NearbyChannels = append(w.NearbyChannels, Nearby{ch, band})
					}
				}
			}
			return w, true
		}
	}
	return WiFi{}, false
}

func detectLinuxWiFi() (WiFi, bool) {
	var w WiFi
	if out, err := exec.Command("iw", "dev").Output(); err == nil {
		for _, ifc := range ParseIwDevInterfaces(string(out)) {
			link, err := exec.Command("iw", "dev", ifc, "link").Output()
			if err != nil {
				continue
			}
			if lw, ok := ParseIwLink(string(link)); ok {
				w = lw
				w.Interface = ifc
				break
			}
		}
	}
	if out, err := exec.Command("nmcli", "-t", "-f", "IN-USE,CHAN,FREQ,SIGNAL", "dev", "wifi", "list", "--rescan", "no").Output(); err == nil {
		cur, nearby := ParseNmcli(string(out))
		w.NearbyChannels = nearby
		if w.Channel == 0 && cur.Channel > 0 {
			w.Channel, w.Band, w.Source = cur.Channel, cur.Band, "nmcli"
		}
		for _, n := range nearby {
			if n.Band == "5GHz" {
				w.Supports5GHz = true // the card can see 5 GHz networks
			}
		}
	}
	if w.Channel == 0 {
		return WiFi{}, false
	}
	if w.Band == "5GHz" || w.Band == "6GHz" {
		w.Supports5GHz = true
	}
	return w, true
}

// ParseIwDevInterfaces lists interface names from `iw dev`.
func ParseIwDevInterfaces(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "Interface" {
			names = append(names, f[1])
		}
	}
	return names
}

// ParseIwLink parses `iw dev <if> link`.
func ParseIwLink(out string) (WiFi, bool) {
	if strings.Contains(out, "Not connected") {
		return WiFi{}, false
	}
	w := WiFi{Source: "iw"}
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		f := strings.Fields(v)
		if len(f) == 0 {
			continue
		}
		switch k {
		case "freq":
			mhz, _ := strconv.ParseFloat(f[0], 64)
			w.Channel, w.Band = ChannelFromFreq(int(mhz))
		case "signal":
			w.SignalDBm, _ = strconv.Atoi(f[0])
		case "tx bitrate":
			r, _ := strconv.ParseFloat(f[0], 64)
			w.TxRateMbps = int(r)
		}
	}
	return w, w.Channel > 0
}

// ParseNmcli parses `nmcli -t -f IN-USE,CHAN,FREQ,SIGNAL dev wifi list`.
func ParseNmcli(out string) (current Nearby, nearby []Nearby) {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, ":")
		if len(f) < 3 {
			continue
		}
		ch, _ := strconv.Atoi(f[1])
		mhz, _ := strconv.Atoi(strings.Fields(f[2] + " 0")[0])
		_, band := ChannelFromFreq(mhz)
		if band == "" {
			band = BandForChannel(ch)
		}
		n := Nearby{Channel: ch, Band: band}
		if strings.TrimSpace(f[0]) == "*" {
			current = n
			continue
		}
		nearby = append(nearby, n)
	}
	return current, nearby
}
