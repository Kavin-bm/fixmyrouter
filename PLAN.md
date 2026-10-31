# fixmyrouter: Plan

## How it works
1. **Discover:** find the default gateway, vendor (from the admin page, or the MAC OUI with `--online`) and admin page fingerprint. *(built)*
2. **Speed (read-only), the headline:** `fixmyrouter speed`
   - Wi-Fi band, channel, width, signal/noise, link rate from the host OS (macOS `system_profiler`, Linux `iw`/`nmcli`). *(built)*
   - Channel congestion from the OS's last scan: neighbours overlapping your channel vs the quietest of 1/6/11 (2.4 GHz) or 36-48/149-161 (5 GHz). *(built)*
   - Gateway ping latency, jitter and loss. *(built)*
   - DNS lookup time: current resolver vs 1.1.1.1, 8.8.8.8, 9.9.9.9. *(built)*
   - Bufferbloat: latency to 1.1.1.1 idle vs during a capped download (opt-in `--load`, up to 200 MB). *(built)*
   - Plain-language suggestions: change channel, use 5 GHz, move router / add mesh node, change DNS, enable SQM/QoS. *(built)*
   - Later: LAN throughput to the router, 6 GHz data, Windows support, per-device "who is hogging bandwidth" if the router exposes it.
3. **Audit (read-only), the security part:** `fixmyrouter audit`
   - Built: admin page over plain HTTP, Telnet port 23 open, UPnP/SSDP answering.
   - Later: WPS enabled, remote management on the WAN side, DNS hijack (unknown resolvers), firmware age if the profile has it, Wi-Fi security mode (WEP/WPA/WPA2/WPA3), other open LAN-side ports.
   - Default-credential or password checks are a **non-goal**: fixmyrouter never sends credentials to any login form, default or otherwise.
4. **Report:** terminal and JSON today; Markdown report later.
5. **Diagnose → fix → measure (built):** the product goal is to *solve* slow internet where it can be solved, not only to point at it, and to be honest where it can't (it cannot raise the ISP's bandwidth).
   - `fixmyrouter diagnose` names the primary bottleneck (Wi-Fi, bufferbloat, DNS, router, ISP) with an impact score, evidence numbers and plain language. The ISP is only blamed at full weight when the local link is healthy. Optional capped download test (`--throughput`, Cloudflare, 4 streams) and `--plan` comparison.
   - Each bottleneck maps to actions by level: **auto** (this computer, e.g. DNS to the fastest *measured* resolver, macOS), **router** (only where the matched profile's `router_fix` allow-list includes the setting), **guided** (profile menu paths), **advice**.
   - `fixmyrouter fix --measure` re-runs the same tests after the fix and compares. A change only counts if it beats measurement noise (e.g. DNS ≥ max(3 ms, 10%), loaded latency ≥ max(10 ms, 15%), download ≥ max(2 Mbps, 10%)); otherwise it says "no measurable gain".
   - `fixmyrouter report --isp` collects timestamped samples into Markdown + JSON, and says when local problems weaken the case.
   - **Router-side changes:** `internal/routerfix` with a driver per documented local API (first: OpenWrt ubus JSON-RPC: Wi-Fi channel, SQM/cake). Gates: profile allow-list, private gateway IP only, the person's typed password (sent once, never stored, never retried, never via the UI or MCP), plan shown first, snapshot before writing, read-back after, `router undo`. Credential guessing and default-password lists remain a non-goal.
   - `fixmyrouter fix` (without `--measure`) still applies the legacy finding-based fix on *this computer* after a yes.
6. **Undo:** `fixmyrouter undo` restores snapshots from `~/.fixmyrouter/changes.json` (`--all` for every change).
7. For unsupported models, the report gives step-by-step manual instructions.

## Agent mode
`PROMPT.md` tells a coding agent how to run `fixmyrouter diagnose --json`, explain the verdict to a non-technical person, ask before any download test and before every fix, and report before/after honestly. `fixmyrouter mcp` exposes the same functions as MCP tools (`diagnose`, `measure_fix`, `isp_report`, plus `check`, `plan_fix`, `apply_fix`, `undo`, `history`); fixes refuse unless the agent confirms the person said yes, and router fixes are refused outright (password typed by the person in a terminal only).

## Repo layout
```
fixmyrouter/
  cmd/fixmyrouter/        CLI entrypoint
  internal/discover/      gateway, OUI, fingerprint
  internal/speed/         Wi-Fi info, congestion, ping, DNS, bufferbloat, suggestions
  internal/checks/        security checks, one file per check
  internal/run/           one entry point shared by CLI, UI and MCP
  internal/diagnose/      bottleneck verdict, fix actions by level, before/after compare
  internal/isp/           ISP evidence report (Markdown + JSON)
  internal/routerfix/     router-side changes behind the profile allow-list (OpenWrt ubus)
  internal/fix/           plan, apply and undo with snapshots (this computer)
  internal/report/        terminal and JSON output
  internal/mcp/           MCP server
  internal/ui/            local web UI (go:embed)
  profiles/               YAML per router model + loader and validator (community PRs)
  PROMPT.md               paste-into-your-agent installer
  README.md, CONTRIBUTING.md, LICENSE (MIT)
```

## Milestones
- **M0-M3 (done, see CHANGELOG):** discover, speed, audit, `check` with a 0-100 score, JSON and exit codes, Windows parsers, DNS fix with snapshot/undo (macOS), local UI, MCP server, profile loader and validator, GoReleaser, install script.
- **M4 (done, unreleased):** diagnose verdict, fix engine (auto/router/guided/advice), `fix --measure`, `report --isp`, throughput test, OpenWrt router driver (unverified), UI and MCP for all of it.
- **Next:** verify the OpenWrt driver on real hardware; first tagged release (Homebrew tap, curl installer, Windows zip); real runs on Linux and Windows; DNS fix on Linux/Windows; upload bufferbloat; WPS / WAN remote admin / firmware age; 20+ verified profiles.
- **Launch:** a personal-story post on X with a real before/after, plus a Show HN (drafts in LAUNCH.md). Ask for router profile PRs in the same post.

## Risks
- Speed advice can be wrong (e.g. the "neighbours" are the user's own mesh nodes, or DFS channels). Keep suggestions conservative and explain why.
- The bufferbloat test uses real data: keep it opt-in and capped.
- Router UIs vary a lot, so launch with read-only reports plus a few profiles, and grow through PRs.
- Safety: strict consent gates, LAN-only scope, no brute forcing.
- Bricking fears: never touch firmware, and snapshot before every change.
- Router drivers are tested against a simulated API only until someone runs them on a real device; profiles stay `verified: false` and the CLI/UI label them until then.
- Over-claiming: measurements are noisy (especially on Wi-Fi and phone hotspots). Compare only the same tests, use noise thresholds, and say "no measurable gain" rather than guess.

## Open questions
- Which routers does Kavin's family actually use (for a real before/after story)?
- Final name and domain check (the speed angle might suit a different name).
