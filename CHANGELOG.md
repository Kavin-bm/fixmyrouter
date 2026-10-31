# Changelog

All notable changes are listed here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [SemVer](https://semver.org/).

## [Unreleased]

### Added
- `diagnose`: names the primary bottleneck (Wi-Fi, bufferbloat, DNS, router or ISP) with an impact score, evidence and plain-language explanation, and fixes ordered auto / router / guided / advice. Notes when it runs over a phone hotspot.
- `fix --measure [id]`: measures, applies the fix after a yes (or walks you through it), measures again with the same tests and prints before/after. Only reports an improvement when it beats measurement-noise thresholds.
- `report --isp --samples N --interval D`: timestamped Markdown + JSON evidence report with the data budget shown up front.
- Opt-in capped download test (`--throughput`, 4 streams from speed.cloudflare.com, 25 MB default, 200 MB max).
- `router apply|undo|history`: Wi-Fi channel and SQM changes on OpenWrt via its ubus API, only for profiles with a `router_fix` allow-list, with the typed admin password (sent once, never stored), a snapshot and undo. The OpenWrt profile is **unverified** (tested against a simulated router).
- UI: Diagnose button with opt-in download tests, verdict card, Fix with confirmation, before/after panel, ISP report download.
- MCP: `diagnose`, `measure_fix`, `isp_report` tools.
- `check` command: speed and security together, with one 0-100 score and findings sorted worst first.
- `speed`: Wi-Fi band, channel, width, signal/noise and link rate; channel congestion and the quietest channel; router latency, jitter and loss; DNS comparison against Cloudflare, Google and Quad9; opt-in bufferbloat test (`--load`).
- `audit`: plain-HTTP admin page, Telnet, UPnP port forwarding (IGD) and Wi-Fi encryption (open/WEP/WPA/WPA2/WPA3).
- `fix`, `undo`, `history`: switch this Mac's DNS to the fastest resolver after a yes, with a snapshot first and a verified undo.
- `ui`: local dashboard on 127.0.0.1 (embedded, no external requests), with a score gauge, before/after, findings, apply-with-confirmation and an undo history.
- `mcp`: stdio MCP server (`check`, `discover`, `plan_fix`, `apply_fix`, `undo`, `history`).
- Router profiles: embedded YAML with strict validation, a JSON schema for editors, `profiles validate|list`, and model-specific `router_steps` on findings.
- Windows support for gateway, ARP, ping, Wi-Fi (`netsh`) and DNS discovery.
- `--json` on every command and stable exit codes (0 ok, 1 error, 2 usage, 3 findings).
- GoReleaser config, `install.sh` with checksum verification, issue forms and a PR template.

### Fixed
- UPnP no longer warns when the router only runs a media server.
- A cancelled run reports errors instead of passes.
- On Linux with systemd-resolved, the DNS comparison measures the real upstream servers, not the caching stub.
- Bufferbloat only counts pings taken while the download is running.

See [AUDIT.md](AUDIT.md) for the full audit.
