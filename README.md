# Your internet is slow. Find out exactly why, fix what can be fixed, and see the numbers before and after.

**fixmyrouter** names the main bottleneck on the connection your family uses every day: **Wi-Fi** (weak signal, 2.4 GHz, crowded channel), **bufferbloat** (lag when someone downloads), **DNS**, **the router**, or **your internet provider**, with the measurements behind the verdict. Then it fixes what is fixable on your side, measures again, and shows you the difference. It's read-only until you say yes, and every change has a one-command undo.

**What it can't do:** make your provider's line faster. If the bottleneck is your ISP, fixmyrouter says so and writes a timestamped report you can send them. It never claims a fix helped unless the before/after numbers show it.

[![CI](https://github.com/Kavin-bm/fixmyrouter/actions/workflows/ci.yml/badge.svg)](https://github.com/Kavin-bm/fixmyrouter/actions/workflows/ci.yml)
![License: MIT](https://img.shields.io/badge/license-MIT-green)
![macOS · Linux · Windows](https://img.shields.io/badge/macOS%20%C2%B7%20Linux%20%C2%B7%20Windows-single%20binary-blue)

![fixmyrouter UI: score gauge, Wi-Fi, router latency, DNS comparison and findings](docs/screenshots/ui-dark.png)

<sub>The real UI on the author's network (a phone hotspot), dark mode. **TODO:** before/after GIF of a real fix on a home router.</sub>

## Quick start

```sh
git clone https://github.com/Kavin-bm/fixmyrouter && cd fixmyrouter
make build && ./bin/fixmyrouter ui        # or: ./bin/fixmyrouter check
```

You need Go 1.22+. These one-liners go live with the first tagged release (**coming soon**; the config and script are in this repo and tested locally):

```sh
brew install Kavin-bm/tap/fixmyrouter                                                  # macOS, Linux
curl -fsSL https://raw.githubusercontent.com/Kavin-bm/fixmyrouter/main/install.sh | sh  # macOS, Linux (verifies SHA-256)
go install github.com/Kavin-bm/fixmyrouter/cmd/fixmyrouter@latest                       # anywhere with Go
```

Windows: download the zip from Releases (coming soon).

## What you get

```text
$ fixmyrouter check
Wi-Fi:      5GHz, channel 149 (80 MHz), 802.11ax, link rate 1200 Mbps, WPA2
Signal:     -37 dBm (excellent), noise -93 dBm, SNR 56 dB
Router:     ping 172.20.10.1 median 11.6 ms, jitter 39.9 ms, loss 0% (20 pings)
DNS lookup time (median of popular sites):
  Your current DNS   fe80::1435:b7ff:fe67:5764%en0    7.3 ms
  Cloudflare         1.1.1.1                        36.2 ms
  ...
⚠️  WARN  Your Wi-Fi link to the router is unstable [medium]
   Router ping: median 11.6 ms, jitter 39.9 ms, 0% loss. A healthy link is under 5 ms with little jitter.
   How to fix: Usually interference or distance: try 5 GHz, a quieter channel, or a cable for your desk.
✅ PASS  Telnet (port 23) [info]
...
Score: 88/100 (Good). 4 pass, 1 warn, 0 fail, 0 could not run.
```

```text
$ fixmyrouter diagnose --throughput --plan 100
Main bottleneck: Wi-Fi (this device to the router). Weak signal (-78 dBm), on 2.4 GHz though this device supports 5 GHz.
  Evidence:
    - Wi-Fi signal: -78 dBm (weak; good is -67 or better)
    - Band: 2.4 GHz, channel 6, 7 neighbour networks on it
    - Router ping: 24 ms median, 18 ms jitter, 3% loss
  How to fix it:
    [router] Move your 2.4GHz Wi-Fi to channel 11 (fixmyrouter does it on the router)
    [guided] Turn on band steering so devices pick 5 GHz on your OpenWrt LuCI
    [advice] Get a stronger signal where you use the internet
$ fixmyrouter fix --measure set-dns
...
DNS lookup                   48 ms      12 ms    -36.0  better
Measured improvement: DNS lookup 48 ms -> 12 ms (-75%).
```
<sub>Illustrative output; your numbers will differ.</sub>

| Command | What it does |
|---|---|
| `fixmyrouter diagnose` | The main bottleneck (Wi-Fi, bufferbloat, DNS, router or ISP) with evidence and fixes ordered by how automatic they are. `--throughput` adds a capped download test (25 MB from Cloudflare by default), `--load` the lag-under-load test, `--plan <Mbps>` compares with what you pay for. |
| `fixmyrouter fix --measure [id]` | Measures, applies the fix (after a yes) or walks you through it, measures again with the same tests, and prints before/after. |
| `fixmyrouter report --isp` | Samples the connection every `--interval` (`--samples N`) and writes a timestamped Markdown + JSON report for your ISP. Shows the data budget and asks first. |
| `fixmyrouter router apply\|undo\|history` | Changes the Wi-Fi channel or turns on SQM **only on routers whose profile allows it** (today: OpenWrt, unverified), with the admin password you type. Snapshot first, `router undo` restores it. |
| `fixmyrouter check` | Everything below, one 0-100 score, worst problems first. `--load` adds the bufferbloat test. |
| `fixmyrouter speed` | Wi-Fi band, channel, width, signal/noise, link rate; channel congestion and the quietest channel; router latency, jitter and loss; your DNS vs Cloudflare, Google and Quad9. |
| `fixmyrouter audit` | Router security: admin page over plain HTTP, Telnet open, UPnP port forwarding (IGD), Wi-Fi encryption (open/WEP/WPA/WPA2/WPA3). Talks only to your router. |
| `fixmyrouter ui` | The dashboard above, on `127.0.0.1` only: Diagnose button, verdict card, fix with a confirmation and a before/after panel, ISP report download, score gauge, findings, undo history. |
| `fixmyrouter fix [id]` | Lists, or applies after a yes, the fixes it can make for you. Today: switch this Mac's DNS to the fastest resolver. |
| `fixmyrouter undo` | Puts back the last change from its snapshot (`--all` for every change). |
| `fixmyrouter history` | Past scores and every change ever made. |
| `fixmyrouter mcp` | MCP server for Claude Code, Codex and Gemini CLI (`diagnose`, `measure_fix`, `isp_report`, `check`, ...). |
| `fixmyrouter profiles validate` | Checks community router profiles (for contributors). |

Every command takes `--json`. Exit codes are stable: `0` nothing to fix, `1` error, `2` usage, `3` found something to fix.

### Fix levels

| Level | Who does it | Today |
|---|---|---|
| **auto** | fixmyrouter, on this computer, after your yes | Switch DNS to the fastest resolver it measured (macOS) |
| **router** | fixmyrouter, on the router, after your yes and your typed password | Wi-Fi channel and SQM on OpenWrt (**unverified profile**: tested against a simulated router only) |
| **guided** | you, with click-by-click steps for your model | Any setting a router profile describes (channel, band steering, DNS, QoS) |
| **advice** | you | Everything else: router placement, cables, restarting, talking to your ISP |

### Platform support

| | macOS | Linux | Windows |
|---|---|---|---|
| Gateway, security checks, DNS, ping | ✅ tested | ✅ parsers unit-tested | ✅ parsers unit-tested |
| Wi-Fi details and neighbour scan | ✅ `system_profiler` | `iw` + `nmcli` | `netsh wlan` |
| Automatic DNS fix + undo | ✅ tested on a real Mac | manual steps (coming soon) | manual steps (coming soon) |

Only macOS has been run end to end so far. Linux and Windows build in CI and their OS-output parsers have unit tests, but nobody has run them on real hardware yet. If you do, please [share a result](https://github.com/Kavin-bm/fixmyrouter/issues/new?template=result.yml).

## Safety promises

- **Read-only until you say yes.** Every check only reads. Changes on this computer (DNS on macOS) need an explicit yes, save a snapshot first, check the change took, and `fixmyrouter undo` puts it back.
- **Router changes only where a profile allows them, only with your password.** fixmyrouter logs in to your router only for `fixmyrouter router` (or `fix --measure` on a router fix), only if your router's profile lists that setting under `router_fix`, and only with the admin password you type. It is sent once to your own gateway, never stored, and never retried; there is no default-password list and no brute forcing. It shows the exact change, saves a snapshot, and `fixmyrouter router undo` restores it. The web UI and MCP tools never handle router passwords. It never touches firmware. For every other router, fixes are step-by-step instructions.
- **Your router only.** Router probes go only to your own default gateway, which comes from your OS routing table. You can't point it at another IP, and it refuses to run if the gateway isn't a private LAN address. The one exception is the UPnP check's standard SSDP discovery packet: like every phone and TV, it goes to the LAN multicast group, and only the gateway's replies are read.
- **No telemetry, no accounts, no cloud.** Outside traffic is only what you'd expect from a speed check: DNS test lookups to 1.1.1.1, 8.8.8.8 and 9.9.9.9, the Cloudflare downloads if you ask for `--throughput` (25 MB by default, max 200) or `--load` (up to 200 MB) or an ISP report, and a MAC-prefix lookup if you pass `discover --online`. Results stay in `~/.fixmyrouter`.
- **The UI is local.** It listens on 127.0.0.1, loads nothing from the internet, and every API call needs a per-run secret token, so other websites can't drive it.

## Paste this into your coding agent

Using Claude Code, Codex, Gemini CLI or Cursor? Paste this:

```text
Use fixmyrouter (https://github.com/Kavin-bm/fixmyrouter) to find out why my internet is slow.
Follow PROMPT.md in that repo exactly: run `fixmyrouter diagnose --json`, explain the verdict and
its numbers to me in plain language, fix what can be fixed only after I say yes, and show me the
before/after. Never ask for or type my router password yourself.
```

Agents that speak MCP can call it directly: `claude mcp add fixmyrouter -- fixmyrouter mcp`. See [PROMPT.md](PROMPT.md).

## How the score works

Start at 100. Each open problem costs points by severity: high 25, medium 12, low 5. Passing checks and checks that couldn't run cost nothing. 90+ is Great, 75+ Good, 50+ Needs work, below that At risk. The thresholds behind each finding (signal below -67 dBm, router jitter above 10 ms, 15+ ms DNS gap, +60 ms under load...) live in [`internal/speed/speed.go`](internal/speed/speed.go). They're a first calibration, and real before/after results will tune them.

## Roadmap

- [x] Speed: Wi-Fi band/channel/signal, congestion, router latency and jitter, DNS comparison, bufferbloat
- [x] Security: plain-HTTP admin, Telnet, UPnP IGD, Wi-Fi encryption
- [x] 0-100 score, `check`, JSON, stable exit codes, local UI, MCP server
- [x] Opt-in fix with snapshot and undo (DNS on macOS)
- [x] Router profiles with validation and editor schema
- [ ] First tagged release: Homebrew tap, curl installer, Windows zip (**coming soon**)
- [ ] Run and fix on real Linux and Windows machines; DNS fix there
- [x] `diagnose`: one verdict (Wi-Fi, bufferbloat, DNS, router, ISP) with evidence and fixes by level
- [x] `fix --measure`: before/after numbers, no claimed gain without a measured one
- [x] Capped multi-stream download test; `report --isp` evidence report
- [x] Router-side fixes behind a profile allow-list (OpenWrt driver: channel, SQM) with snapshot and undo
- [ ] Verify the OpenWrt driver on a real device; more drivers only for documented, local APIs
- [ ] Upload speed and upload bufferbloat; DNS fix on the router itself (DHCP DNS) as a router-level fix
- [ ] Linux/Windows automatic DNS fix
- [ ] More security checks: WPS, WAN-side remote admin, firmware age (from profiles)
- [ ] 20+ verified router profiles, starting with Indian ISPs (JioFiber, Airtel Xstream) and TP-Link, D-Link, Netgear
- [ ] Before/after share card in the UI

## Contributing

The most useful contribution needs no Go: **add your router**. Copy one YAML file, fill in where each setting lives, and run `fixmyrouter profiles validate profiles`. See [profiles/README.md](profiles/README.md), or use the [Add my router](https://github.com/Kavin-bm/fixmyrouter/issues/new?template=router-profile.yml) form. Code contributions: [CONTRIBUTING.md](CONTRIBUTING.md). Starter tasks: [GOOD_FIRST_ISSUES.md](GOOD_FIRST_ISSUES.md).

### Contributors

<a href="https://github.com/Kavin-bm/fixmyrouter/graphs/contributors"><img src="https://contrib.rocks/image?repo=Kavin-bm/fixmyrouter" alt="Contributors"></a>

## License

MIT, see [LICENSE](LICENSE). Security issues: see [SECURITY.md](SECURITY.md).
