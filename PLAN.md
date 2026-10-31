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
   - Default-credential checks are deliberately **not** in scope for now. If ever added: a tiny vendor-default list, own gateway only, explicit consent, never brute force.
4. **Report:** terminal and JSON today; Markdown report later.
5. **Fix (opt-in, coming soon):** for models with a profile, apply each suggestion (channel, DNS, QoS, UPnP off...) one at a time after a yes. Snapshot the old setting first.
6. **Undo (coming soon):** `fixmyrouter undo` restores every snapshot from `~/.fixmyrouter/`.
7. For unsupported models, the report gives step-by-step manual instructions.

## Agent mode
`PROMPT.md` tells a coding agent how to run `fixmyrouter speed --json` and `audit --json`, explain the results to a non-technical person, ask before running `--load`, and never change settings itself. Later the binary also exposes the same functions as an MCP server (`fixmyrouter mcp`, coming soon).

## Repo layout
```
fixmyrouter/
  cmd/fixmyrouter/        CLI entrypoint
  internal/discover/      gateway, OUI, fingerprint
  internal/speed/         Wi-Fi info, congestion, ping, DNS, bufferbloat, suggestions
  internal/checks/        security checks, one file per check
  internal/profiles/      loader for model profiles (coming soon)
  internal/fix/           apply and undo with snapshots (coming soon)
  internal/report/        terminal and JSON output
  internal/mcp/           MCP server (coming soon)
  profiles/               YAML per router model (community PRs)
  PROMPT.md               paste-into-your-agent installer
  README.md, CONTRIBUTING.md, LICENSE (MIT)
```

## Milestones
- **M0 (day 1):** repo, license, README, `discover`, `speed` (Wi-Fi info, congestion, gateway ping, DNS, opt-in bufferbloat, suggestions), `audit` with 3 security checks. *(done)*
- **M1 (days 2-3):** calibrate speed thresholds on real homes (Kavin's family routers), Markdown report, before/after compare (`speed --save` / `speed --compare`), 3 more security checks.
- **M2 (days 4-5):** profile format and loader, 3 router profiles (e.g. TP-Link, JioFiber, Airtel) covering channel, DNS, QoS and UPnP settings; `fix` and `undo` with snapshots.
- **M3 (day 6):** MCP server, demo GIF, before/after screenshots of a real speed improvement.
- **M4 (day 7):** release binaries (Mac/Linux/Windows) via GoReleaser, Windows Wi-Fi info via `netsh wlan`.
- **Launch:** a personal-story post on X with a before/after speed report ("my parents' Wi-Fi went from X to Y"), plus a Show HN. Ask for router model PRs in the same post.

## Risks
- Speed advice can be wrong (e.g. the "neighbours" are the user's own mesh nodes, or DFS channels). Keep suggestions conservative and explain why.
- The bufferbloat test uses real data: keep it opt-in and capped.
- Router UIs vary a lot, so launch with read-only reports plus a few profiles, and grow through PRs.
- Safety: strict consent gates, LAN-only scope, no brute forcing.
- Bricking fears: never touch firmware, and snapshot before every change.

## Open questions
- Which routers does Kavin's family actually use (for a real before/after story)?
- Final name and domain check (the speed angle might suit a different name).
