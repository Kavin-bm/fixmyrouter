package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
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
	if n := len(r[1]["result"].(map[string]any)["tools"].([]any)); n != 6 {
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
