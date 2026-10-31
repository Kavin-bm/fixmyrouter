package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Kavin-bm/fixmyrouter/internal/diagnose/diagtest"
)

func TestProtocol(t *testing.T) {
	t.Setenv("FIXMYROUTER_HOME", t.TempDir())
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"apply_fix","arguments":{"finding_id":"dns-speed","user_confirmed":false}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"history","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"nope"}`,
	}, "\n")
	var out bytes.Buffer
	if err := Serve(context.Background(), "test", strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("want 5 responses (notification gets none), got %d:\n%s", len(lines), out.String())
	}
	var r []map[string]any
	for _, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(err)
		}
		r = append(r, m)
	}
	if v := r[0]["result"].(map[string]any)["protocolVersion"]; v != "2025-06-18" {
		t.Errorf("version %v", v)
	}
	if n := len(r[1]["result"].(map[string]any)["tools"].([]any)); n != 9 {
		t.Errorf("%d tools", n)
	}
	if res := r[2]["result"].(map[string]any); res["isError"] != true || !strings.Contains(lines[2], "refused") {
		t.Errorf("apply_fix without confirmation must refuse: %s", lines[2])
	}
	if res := r[3]["result"].(map[string]any); res["isError"] != false {
		t.Errorf("history: %s", lines[3])
	}
	if r[4]["error"] == nil {
		t.Error("unknown method should error")
	}
}

func call(t *testing.T, name, args string) (string, bool) {
	t.Helper()
	in := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `","arguments":` + args + `}}`
	var out bytes.Buffer
	if err := Serve(context.Background(), "test", strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	var m struct {
		Result struct {
			Content []struct{ Text string } `json:"content"`
			IsError bool                    `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	return m.Result.Content[0].Text, m.Result.IsError
}

func TestSolveTools(t *testing.T) {
	t.Setenv("FIXMYROUTER_HOME", t.TempDir())
	f := diagtest.Healthy()
	f.W.Band, f.W.Channel, f.W.TxRateMbps = "2.4GHz", 6, 144
	old, oldSleep := Prober, Sleep
	Prober, Sleep, Settle = f, func(context.Context, time.Duration) error { return nil }, 0
	t.Cleanup(func() { Prober, Sleep = old, oldSleep })

	if out, isErr := call(t, "measure_fix", `{"fix_id":"wifi-join-5g","user_confirmed":true}`); !isErr || !strings.Contains(out, "run diagnose first") {
		t.Fatalf("measure_fix before diagnose: %s", out)
	}
	if out, isErr := call(t, "diagnose", `{}`); isErr || !strings.Contains(out, "wifi-join-5g") {
		t.Fatalf("diagnose: %s", out)
	}
	if f.Count("tput")+f.Count("bloat") != 0 {
		t.Fatal("download test without consent")
	}
	if out, isErr := call(t, "measure_fix", `{"fix_id":"set-dns","user_confirmed":false}`); !isErr {
		t.Fatalf("unconfirmed: %s", out)
	}
	f.W.Band, f.W.TxRateMbps = "5GHz", 866
	if out, isErr := call(t, "measure_fix", `{"fix_id":"wifi-join-5g","user_confirmed":true}`); isErr || !strings.Contains(out, "Measured improvement") {
		t.Fatalf("measure_fix: %s", out)
	}
	if out, isErr := call(t, "isp_report", `{"samples":2,"interval_seconds":10}`); !isErr || !strings.Contains(out, "50 MB") {
		t.Fatalf("isp_report without consent: %s", out)
	}
	if out, isErr := call(t, "isp_report", `{"samples":10,"interval_seconds":600,"no_download":true}`); !isErr || !strings.Contains(out, "at most") {
		t.Fatalf("too long: %s", out)
	}
	if out, isErr := call(t, "isp_report", `{"samples":2,"interval_seconds":10,"no_download":true}`); isErr || !strings.Contains(out, "markdown_file") {
		t.Fatalf("isp_report: %s", out)
	}
}
