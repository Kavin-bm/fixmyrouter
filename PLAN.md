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
5. **Fix (opt-in):** `fixmyrouter fix` applies changes on *this computer* after a yes, with a snapshot first (today: DNS on macOS). Router settings are never changed automatically, because that would mean logging in to the router; profiles give exact manual steps instead.
6. **Undo:** `fixmyrouter undo` restores snapshots from `~/.fixmyrouter/changes.json` (`--all` for every change).
7. For unsupported models, the report gives step-by-step manual instructions.

## Agent mode
`PROMPT.md` tells a coding agent how to run `fixmyrouter check --json`, explain the results to a non-technical person, ask before `--load` and before every fix. `fixmyrouter mcp` exposes the same functions as MCP tools; `apply_fix` refuses unless the agent confirms the person said yes.

## Repo layout
```
fixmyrouter/
  cmd/fixmyrouter/        CLI entrypoint
  internal/discover/      gateway, OUI, fingerprint
  internal/speed/         Wi-Fi info, congestion, ping, DNS, bufferbloat, suggestions
  internal/checks/        security checks, one file per check
  internal/run/           one entry point shared by CLI, UI and MCP
  internal/fix/           plan, apply and undo with snapshots
  internal/report/        terminal and JSON output
  internal/mcp/           MCP server
  internal/ui/            local web UI (go:embed)
  profiles/               YAML per router model + loader and validator (community PRs)
  PROMPT.md               paste-into-your-agent installer
  README.md, CONTRIBUTING.md, LICENSE (MIT)
```

## Milestones
- **M0-M3 (done, see CHANGELOG):** discover, speed, audit, `check` with a 0-100 score, JSON and exit codes, Windows parsers, DNS fix with snapshot/undo (macOS), local UI, MCP server, profile loader and validator, GoReleaser, install script.
- **Next:** first tagged release (Homebrew tap, curl installer, Windows zip); real runs on Linux and Windows; DNS fix on Linux/Windows; upload bufferbloat; WPS / WAN remote admin / firmware age; 20+ verified profiles.
- **Launch:** a personal-story post on X with a real before/after, plus a Show HN (drafts in LAUNCH.md). Ask for router profile PRs in the same post.

## Risks
- Speed advice can be wrong (e.g. the "neighbours" are the user's own mesh nodes, or DFS channels). Keep suggestions conservative and explain why.
- The bufferbloat test uses real data: keep it opt-in and capped.
- Router UIs vary a lot, so launch with read-only reports plus a few profiles, and grow through PRs.
- Safety: strict consent gates, LAN-only scope, no brute forcing.
- Bricking fears: never touch firmware, and snapshot before every change.

## Open questions
- Which routers does Kavin's family actually use (for a real before/after story)?
- Final name and domain check (the speed angle might suit a different name).
