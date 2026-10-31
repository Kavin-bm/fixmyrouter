package routerfix

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// openWrt talks to OpenWrt's documented ubus JSON-RPC endpoint (/ubus), the
// same API LuCI uses. https://openwrt.org/docs/techref/ubus
type openWrt struct{}

func init() { register(openWrt{}) }

func (openWrt) Name() string { return "openwrt-ubus" }

const nullSession = "00000000000000000000000000000000"

type ubus struct {
	url    string
	client *http.Client
	sid    string
	id     atomic.Int64
}

// Login sends the person's password once. A wrong password is an error,
// never a retry.
func (openWrt) Login(ctx context.Context, baseURL, user, password string) (Session, error) {
	u := &ubus{
		url: strings.TrimSuffix(baseURL, "/") + "/ubus",
		client: &http.Client{
			Timeout: 15 * time.Second,
			// Routers use self-signed certificates. We only ever connect to
			// the private gateway IP (see BaseURL).
			Transport:     &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		sid: nullSession,
	}
	res, err := u.call(ctx, "session", "login", map[string]any{"username": user, "password": password})
	if err != nil {
		if strings.Contains(err.Error(), "permission denied") {
			return nil, fmt.Errorf("the router rejected the login (wrong user or password). fixmyrouter does not retry; run the command again to try once more")
		}
		return nil, err
	}
	sid, _ := res["ubus_rpc_session"].(string)
	if sid == "" {
		return nil, fmt.Errorf("router login returned no session")
	}
	u.sid = sid
	return u, nil
}

var ubusStatus = map[float64]string{
	1: "invalid command", 2: "invalid argument", 3: "method not found", 4: "not found",
	5: "no data", 6: "permission denied", 7: "timeout", 8: "not supported", 9: "unknown error", 10: "connection failed",
}

func (u *ubus) call(ctx context.Context, object, method string, args any) (map[string]any, error) {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": u.id.Add(1), "method": "call",
		"params": []any{u.sid, object, method, args},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("router answered HTTP %d on /ubus (is this OpenWrt with LuCI?)", resp.StatusCode)
	}
	var r struct {
		Result []json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("router sent an unexpected reply: %w", err)
	}
	if r.Error != nil {
		if r.Error.Code == -32002 {
			return nil, fmt.Errorf("%s.%s: permission denied", object, method)
		}
		return nil, fmt.Errorf("%s.%s: %s", object, method, r.Error.Message)
	}
	if len(r.Result) == 0 {
		return nil, fmt.Errorf("%s.%s: empty reply", object, method)
	}
	var status float64
	_ = json.Unmarshal(r.Result[0], &status)
	if status != 0 {
		msg := ubusStatus[status]
		if msg == "" {
			msg = "status " + strconv.Itoa(int(status))
		}
		return nil, fmt.Errorf("%s.%s: %s", object, method, msg)
	}
	out := map[string]any{}
	if len(r.Result) > 1 {
		_ = json.Unmarshal(r.Result[1], &out)
	}
	return out, nil
}

func (u *ubus) Sections(ctx context.Context, config string) (map[string]map[string]string, error) {
	res, err := u.call(ctx, "uci", "get", map[string]any{"config": config})
	if err != nil {
		return nil, err
	}
	vals, _ := res["values"].(map[string]any)
	out := map[string]map[string]string{}
	for name, v := range vals {
		opts, _ := v.(map[string]any)
		m := map[string]string{}
		for k, ov := range opts {
			switch x := ov.(type) {
			case string:
				m[k] = x
			case []any:
				var parts []string
				for _, p := range x {
					parts = append(parts, fmt.Sprint(p))
				}
				m[k] = strings.Join(parts, " ")
			default:
				m[k] = fmt.Sprint(x)
			}
		}
		out[name] = m
	}
	return out, nil
}

func (u *ubus) Get(ctx context.Context, config, section, option string) (string, bool, error) {
	res, err := u.call(ctx, "uci", "get", map[string]any{"config": config, "section": section, "option": option})
	if err != nil {
		if strings.HasSuffix(err.Error(), "not found") {
			return "", false, nil
		}
		return "", false, err
	}
	v, ok := res["value"].(string)
	return v, ok, nil
}

func (u *ubus) Set(ctx context.Context, config, section, option, value string) error {
	_, err := u.call(ctx, "uci", "set", map[string]any{"config": config, "section": section, "values": map[string]string{option: value}})
	return err
}

func (u *ubus) Delete(ctx context.Context, config, section, option string) error {
	_, err := u.call(ctx, "uci", "delete", map[string]any{"config": config, "section": section, "option": option})
	return err
}

// Commit saves the config; rpcd then signals config.change so OpenWrt
// reloads Wi-Fi or SQM (a few seconds of Wi-Fi drop for channel changes).
func (u *ubus) Commit(ctx context.Context, config string) error {
	_, err := u.call(ctx, "uci", "commit", map[string]any{"config": config})
	return err
}

var channels2g = map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true, 9: true, 10: true, 11: true, 12: true, 13: true}
var channels5g = map[int]bool{36: true, 40: true, 44: true, 48: true, 52: true, 56: true, 60: true, 64: true, 100: true, 104: true, 108: true, 112: true, 116: true, 120: true, 124: true, 128: true, 132: true, 136: true, 140: true, 144: true, 149: true, 153: true, 157: true, 161: true, 165: true}

// radioBand tells which band an OpenWrt wifi-device section is on.
func radioBand(o map[string]string) string {
	switch {
	case o["band"] == "2g" || o["hwmode"] == "11g" || o["hwmode"] == "11b" || o["hwmode"] == "11ng":
		return "2g"
	case o["band"] == "5g" || o["hwmode"] == "11a" || o["hwmode"] == "11na" || o["hwmode"] == "11ac":
		return "5g"
	}
	if ch, err := strconv.Atoi(o["channel"]); err == nil {
		if channels2g[ch] {
			return "2g"
		}
		if channels5g[ch] {
			return "5g"
		}
	}
	return ""
}

func (openWrt) Plan(ctx context.Context, s Session, setting, value string) (string, []Edit, error) {
	switch setting {
	case "wifi_channel_2g", "wifi_channel_5g":
		band, valid := "2g", channels2g
		if setting == "wifi_channel_5g" {
			band, valid = "5g", channels5g
		}
		ch, err := strconv.Atoi(value)
		if err != nil || !valid[ch] {
			return "", nil, fmt.Errorf("%q is not a valid %s channel", value, map[string]string{"2g": "2.4 GHz", "5g": "5 GHz"}[band])
		}
		secs, err := s.Sections(ctx, "wireless")
		if err != nil {
			return "", nil, err
		}
		var names []string
		for name, o := range secs {
			if o[".type"] == "wifi-device" && radioBand(o) == band {
				names = append(names, name)
			}
		}
		if len(names) == 0 {
			return "", nil, fmt.Errorf("no %s radio found in /etc/config/wireless", map[string]string{"2g": "2.4 GHz", "5g": "5 GHz"}[band])
		}
		sort.Strings(names)
		var edits []Edit
		for _, n := range names {
			cur, ok := secs[n]["channel"]
			if ok && cur == value {
				continue
			}
			edits = append(edits, Edit{Config: "wireless", Section: n, Option: "channel", Before: cur, BeforeSet: ok, After: value})
		}
		if len(edits) == 0 {
			return "", nil, fmt.Errorf("the %s radio is already on channel %s", map[string]string{"2g": "2.4 GHz", "5g": "5 GHz"}[band], value)
		}
		return fmt.Sprintf("Set the %s Wi-Fi channel to %s (Wi-Fi drops for a few seconds while the radio restarts).", map[string]string{"2g": "2.4 GHz", "5g": "5 GHz"}[band], value), edits, nil
	case "sqm_qos":
		down, up, err := parseRates(value)
		if err != nil {
			return "", nil, err
		}
		secs, err := s.Sections(ctx, "sqm")
		if err != nil {
			return "", nil, fmt.Errorf("SQM is not installed on this router (install luci-app-sqm in System > Software, then try again): %w", err)
		}
		var names []string
		for name, o := range secs {
			if o[".type"] == "queue" {
				names = append(names, name)
			}
		}
		if len(names) == 0 {
			return "", nil, fmt.Errorf("no SQM queue configured; open Network > SQM QoS once to create one")
		}
		sort.Strings(names)
		q := names[0]
		if secs[q]["interface"] == "" {
			return "", nil, fmt.Errorf("the SQM queue has no interface; pick your WAN interface in Network > SQM QoS first")
		}
		want := [][2]string{{"enabled", "1"}, {"download", trimKbit(down)}, {"upload", trimKbit(up)}, {"qdisc", "cake"}, {"script", "piece_of_cake.qos"}}
		var edits []Edit
		for _, kv := range want {
			cur, ok := secs[q][kv[0]]
			if ok && cur == kv[1] {
				continue
			}
			edits = append(edits, Edit{Config: "sqm", Section: q, Option: kv[0], Before: cur, BeforeSet: ok, After: kv[1]})
		}
		if len(edits) == 0 {
			return "", nil, fmt.Errorf("SQM is already on with these limits")
		}
		upText := "upload not shaped"
		if up > 0 {
			upText = fmt.Sprintf("upload %.0f Mbps", up)
		}
		return fmt.Sprintf("Turn on SQM (cake) on %s: download %.0f Mbps, %s. Keeps latency low when the line is busy; peak speed drops slightly.", secs[q]["interface"], down, upText), edits, nil
	}
	return "", nil, fmt.Errorf("openwrt-ubus cannot change %q", setting)
}

// parseRates reads "DOWN" or "DOWN/UP" in Mbps.
func parseRates(v string) (down, up float64, err error) {
	d, u, hasUp := strings.Cut(v, "/")
	down, err = strconv.ParseFloat(strings.TrimSpace(d), 64)
	if err != nil || down < 1 || down > 10000 {
		return 0, 0, fmt.Errorf("SQM value must be DOWN or DOWN/UP in Mbps, e.g. 90 or 90/18 (got %q)", v)
	}
	if hasUp {
		up, err = strconv.ParseFloat(strings.TrimSpace(u), 64)
		if err != nil || up < 0 || up > 10000 {
			return 0, 0, fmt.Errorf("SQM upload must be a number of Mbps (got %q)", u)
		}
	}
	return down, up, nil
}
