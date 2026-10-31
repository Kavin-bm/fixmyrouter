# Your Wi-Fi is slower than it should be. Your coding agent fixes it.

> One command measures why your home Wi-Fi feels slow (crowded channel, 2.4 GHz, weak signal, slow DNS, lag under load) and tells you exactly what to change, in plain English. It also checks your router for risky settings. Read-only: it never changes anything.

<!-- TODO: before/after screenshot or demo GIF of `fixmyrouter speed` goes here -->
**TODO: demo GIF (before/after speed report)**

## Quick start

```sh
go run github.com/kavinbm/fixmyrouter/cmd/fixmyrouter@latest speed
```

> TODO: the module path `github.com/kavinbm/fixmyrouter` is a placeholder until the GitHub repo exists. Prebuilt binaries and Homebrew are coming soon.

Or from a clone: `make build && ./bin/fixmyrouter speed`

## What it checks today

### `fixmyrouter speed` (macOS and Linux)

| Measurement | How |
|---|---|
| Wi-Fi band, channel, width, signal, noise, link rate | Read from the OS (`system_profiler` on macOS, `iw` / `nmcli` on Linux) |
| Channel congestion | Counts nearby networks the OS already scanned that overlap your channel, and finds the quietest standard channel (1/6/11 on 2.4 GHz; 36-48 or 149-161 on 5 GHz). Network names are never stored. |
| Router latency and jitter | 20 pings to your own default gateway |
| DNS speed | Times lookups of popular sites on your current DNS vs Cloudflare (1.1.1.1), Google (8.8.8.8) and Quad9 (9.9.9.9) |
| Bufferbloat (opt-in, `--load`) | Pings 1.1.1.1 idle, then while downloading test data from Cloudflare for ~10 s (uses up to 200 MB, so it's off by default) |

Then it prints suggestions such as **switch to channel 1**, **use 5 GHz**, **move the router / add a mesh node**, **change DNS to 1.1.1.1**, or **turn on SQM/QoS**. `--json` gives the same report to coding agents.

### `fixmyrouter audit`: security checks (read-only)

- **Admin page encryption:** is the router login served over plain HTTP?
- **Telnet (port 23):** is the clear-text remote shell IoT botnets scan for left open?
- **UPnP / SSDP:** can any device on your network open ports to the internet without asking?

### `fixmyrouter discover`

Your default gateway, its MAC address, and the vendor when the admin page or (with `--online`) the MAC prefix reveals it.

### Coming soon (not built yet)

- `fixmyrouter fix`: apply suggestions one at a time on routers with a profile, after a yes, with a snapshot first
- `fixmyrouter undo`: restore every snapshot with one command
- `fixmyrouter mcp`: MCP server so coding agents can call the checks directly
- Throughput test against the router, 6 GHz scan data, Windows support, release binaries
- More security checks: WPS, DNS hijack, WAN-side remote admin, firmware age

## Safety promises

- **Read-only today.** fixmyrouter changes nothing on your router or computer. Future fixes will ask before each change and snapshot the old setting first, with a one-command undo.
- **Your router only.** It only probes your own default gateway, and refuses to run if that isn't a private LAN address.
- **No password guessing.** No brute forcing, no default-credential attempts, nothing sent to the login form.
- **No firmware changes, ever.**
- **No telemetry.** The only outside traffic is DNS test lookups, plus the Cloudflare download if you pass `--load` and the MAC-prefix lookup if you pass `--online`.

## Paste this into your coding agent

Using Claude Code, Codex, Cursor or another coding agent? Paste this:

```text
Use fixmyrouter (https://github.com/kavinbm/fixmyrouter) to find out why my Wi-Fi is slow.
Follow PROMPT.md in that repo exactly: run `fixmyrouter speed --json` (and `audit --json`),
explain the results in plain language, walk me through each suggestion step by step,
and do not change any router setting yourself.
```

See [PROMPT.md](PROMPT.md) for the full agent instructions.

## Add your router (we need your help!)

Every router UI is different, so fixmyrouter grows through **router profiles**: one small YAML file per model in [`profiles/`](profiles/) saying where each setting lives (channel, band steering, DNS, QoS/SQM, UPnP...). If you own a TP-Link, JioFiber, Airtel Xstream, Netgear, ASUS or any other router, a profile PR is the most useful contribution you can make. Template: [profiles/README.md](profiles/README.md). Guide: [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT, see [LICENSE](LICENSE).
