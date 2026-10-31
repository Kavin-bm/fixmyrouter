// Package mcp is a minimal Model Context Protocol server over stdio
// (newline-delimited JSON-RPC 2.0), so coding agents can call the same
// functions as the CLI. Read-only tools run freely; apply_fix and
// measure_fix refuse unless the agent passes user_confirmed=true after asking
// the person. No tool logs in to the router.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/Kavin-bm/fixmyrouter/internal/discover"
	"github.com/Kavin-bm/fixmyrouter/internal/fix"
	"github.com/Kavin-bm/fixmyrouter/internal/run"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	call        func(ctx context.Context, args map[string]any) (any, error)
}

func schema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func tools(version string) []tool {
	str := func(desc string, enum ...string) map[string]any {
		m := map[string]any{"type": "string", "description": desc}
		if len(enum) > 0 {
			m["enum"] = enum
		}
		return m
	}
	return append(solveTools(version), []tool{
		{
			Name: "check",
			Description: "Measure the home network and return a 0-100 score with plain-language findings. kind=check (everything), speed (Wi-Fi, router latency, DNS) or audit (router security, talks only to the router). " +
				"Read-only. load=true adds a bufferbloat test that downloads up to 200 MB: ask the person first.",
			InputSchema: schema(map[string]any{
				"kind": str("check, speed or audit (default check)", run.KindCheck, run.KindSpeed, run.KindAudit),
				"load": map[string]any{"type": "boolean", "description": "bufferbloat test, downloads up to 200 MB (ask first)"},
			}),
			call: func(ctx context.Context, a map[string]any) (any, error) {
				kind, _ := a["kind"].(string)
				if kind == "" {
					kind = run.KindCheck
				}
				if kind != run.KindCheck && kind != run.KindSpeed && kind != run.KindAudit {
					return nil, fmt.Errorf("kind must be check, speed or audit")
				}
				load, _ := a["load"].(bool)
				r, err := run.Run(ctx, version, run.Options{Kind: kind, Load: load && kind != run.KindAudit})
				if err != nil {
					return nil, err
				}
				out := map[string]any{"result": r}
				if prev := run.Previous(r); prev != nil {
					out["previous_score"] = prev.Score
				}
				_ = run.Save(r)
				return out, nil
			},
		},
		{
			Name:        "discover",
			Description: "Show the default gateway (the router), its MAC address and vendor if known. Read-only.",
			InputSchema: schema(map[string]any{}),
			call: func(ctx context.Context, _ map[string]any) (any, error) {
				return discover.Run(ctx, false)
			},
		},
		{
			Name:        "plan_fix",
			Description: "Show exactly what an automatic fix would change (before and after), without changing anything. Use the finding id from a check result whose auto_fix is set.",
			InputSchema: schema(map[string]any{"finding_id": str("finding id, e.g. dns-speed")}, "finding_id"),
			call: func(_ context.Context, a map[string]any) (any, error) {
				return planFix(a)
			},
		},
		{
			Name: "apply_fix",
			Description: "Apply an automatic fix after saving a snapshot. ONLY call this after showing the person the plan_fix result and getting an explicit yes; " +
				"set user_confirmed=true to say you did. Every change can be reverted with undo.",
			InputSchema: schema(map[string]any{
				"finding_id":     str("finding id, e.g. dns-speed"),
				"user_confirmed": map[string]any{"type": "boolean", "description": "true only if the person explicitly said yes to this change"},
			}, "finding_id", "user_confirmed"),
			call: func(_ context.Context, a map[string]any) (any, error) {
				if ok, _ := a["user_confirmed"].(bool); !ok {
					return nil, fmt.Errorf("refused: ask the person first, then call again with user_confirmed=true")
				}
				c, err := planFix(a)
				if err != nil {
					return nil, err
				}
				if err := fix.Apply(c); err != nil {
					return c, fmt.Errorf("change %d failed: %w; call undo with change_id %d to make sure the old setting is back", c.ID, err, c.ID)
				}
				return c, nil
			},
		},
		{
			Name:        "undo",
			Description: "Put back a change from its snapshot: change_id, or the latest change if omitted.",
			InputSchema: schema(map[string]any{"change_id": map[string]any{"type": "integer", "description": "change id from history; omit for the latest"}}),
			call: func(_ context.Context, a map[string]any) (any, error) {
				id, _ := a["change_id"].(float64)
				return fix.Undo(int(id))
			},
		},
		{
			Name:        "history",
			Description: "Past scores and every change fixmyrouter has made on this computer.",
			InputSchema: schema(map[string]any{}),
			call: func(_ context.Context, _ map[string]any) (any, error) {
				runs, err := run.History()
				if err != nil {
					return nil, err
				}
				changes, err := fix.History()
				scores := []map[string]any{}
				for _, r := range runs {
					scores = append(scores, map[string]any{"kind": r.Kind, "generated_at": r.GeneratedAt, "score": r.Score})
				}
				return map[string]any{"scores": scores, "changes": changes}, err
			},
		},
	}...)
}

func planFix(a map[string]any) (*fix.Change, error) {
	id, _ := a["finding_id"].(string)
	f, _ := run.FindAutoFix(id)
	if f == nil {
		return nil, fmt.Errorf("no automatic fix %q in saved results; run the check tool first", id)
	}
	_, iface, err := discover.DefaultGateway()
	if err != nil {
		return nil, err
	}
	return fix.Plan(f.AutoFix, iface)
}

// Serve answers requests from r on w until r is closed.
func Serve(ctx context.Context, version string, r io.Reader, w io.Writer) error {
	ts := tools(version)
	byName := map[string]tool{}
	for _, t := range ts {
		byName[t.Name] = t
	}
	enc := json.NewEncoder(w)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var req request
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			_ = enc.Encode(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}})
			continue
		}
		if len(req.ID) == 0 {
			continue // notification, e.g. notifications/initialized
		}
		resp := response{JSONRPC: "2.0", ID: req.ID}
		switch req.Method {
		case "initialize":
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if p.ProtocolVersion == "" {
				p.ProtocolVersion = "2025-06-18"
			}
			resp.Result = map[string]any{
				"protocolVersion": p.ProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "fixmyrouter", "version": version},
				"instructions":    "Start with diagnose when the internet is slow; explain the verdict and its numbers in plain language. fixmyrouter cannot make the ISP faster: it fixes what is fixable on the person's side and measures before and after. Never call apply_fix or measure_fix for an automatic fix without the person's explicit yes, and never start a download test or isp_report download without telling them the data use. Report before/after verdicts as given; do not claim an improvement the numbers don't show. Router settings are never changed by these tools: router fixes need the person's own password typed in a terminal.",
			}
		case "ping":
			resp.Result = map[string]any{}
		case "tools/list":
			resp.Result = map[string]any{"tools": ts}
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			if err := json.Unmarshal(req.Params, &p); err != nil {
				resp.Error = &rpcError{-32602, "invalid params"}
				break
			}
			t, ok := byName[p.Name]
			if !ok {
				resp.Error = &rpcError{-32602, "unknown tool " + p.Name}
				break
			}
			out, err := t.call(ctx, p.Arguments)
			resp.Result = toolResult(out, err)
		default:
			resp.Error = &rpcError{-32601, "method not found: " + req.Method}
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return sc.Err()
}

func toolResult(out any, err error) map[string]any {
	text := ""
	if out != nil {
		b, _ := json.MarshalIndent(out, "", "  ")
		text = string(b)
	}
	if err != nil {
		text = "error: " + err.Error() + "\n" + text
	}
	return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}, "isError": err != nil}
}
