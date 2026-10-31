# fixmyrouter audit (Phase 1)

Date: 2026-10-31. Scope: the whole repository at commit `29bd848` (CLI, `internal/checks`, `internal/discover`, `internal/speed`, `internal/report`, docs, CI).

Tools run on macOS 26 (arm64), Go 1.26.3:

| Tool | Result before fixes | Result after fixes |
|---|---|---|
| `go build ./...` | ok | ok |
| `go vet ./...` | no issues | no issues |
| `go test -race ./...` | 4 packages ok | 4 packages ok, 2 new tests |
| `staticcheck ./...` | no issues | no issues |
| `gofmt -l .` | clean | clean |

Note: a second agent session was editing this repo at the same time as this audit. It amended the initial commit (`29bd848`) and that commit already contains the fixes marked **(in 29bd848)** below, which this audit wrote before the amend. Everything else is fixed in the Phase 1 commit.

## Safety confirmation

These are the promises the project makes. Each was checked against the code, not the docs.

| Promise | Verified how | Result |
|---|---|---|
| Never brute forces or tries default credentials | Searched all Go code for credential, password, `Authorization`, `SetBasicAuth`, POST requests. The only HTTP requests are one unauthenticated `GET /` to the gateway (fingerprint and http-admin check), the opt-in Cloudflare download (`speed --load`) and the opt-in MAC-vendor lookup (`discover --online`). | **Holds.** No code sends a username or password anywhere. *Update (unreleased):* `internal/routerfix` now sends the admin password the person types, once, to the private gateway, only for settings a profile's `router_fix` allow-list includes; never stored, never retried, no default-password list, not reachable from the UI or MCP. |
| Only talks to the user's own default gateway | The gateway IP comes only from the OS routing table (`route -n get default`, `/proc/net/route`); no command accepts an IP from the user; non-private gateways are refused. | **Holds for router probes**, with two documented exceptions: the UPnP check sends the standard SSDP multicast (reaches all LAN devices, only gateway replies are read), and `speed` contacts public DNS resolvers and, with `--load`, Cloudflare. README wording was too absolute and is fixed. |
| Never changes anything without explicit consent and a snapshot | No code path writes to the router or the OS. | **Holds** (nothing can change anything yet). Phase 2 adds the first change (local DNS), which must go through consent + snapshot. |

## Findings

Severity: **high** = wrong or unsafe result a user would act on; **medium** = misleading output, a broken contract, or a real gap in testing/CI; **low** = edge case, polish or a known limitation.

| # | Severity | Area | Finding | Status |
|---|---|---|---|---|
| 1 | medium | checks/upnp | The UPnP check warned on *any* SSDP reply from the gateway, so a router that only runs a DLNA media server was told "apps can open ports to the internet" (false positive). | **Fixed (in 29bd848):** searches for the InternetGatewayDevice service and warns only if the port-forwarding service answers; root-device-only replies pass with an explanation. Test added. |
| 2 | medium | checks | Errors were reported as passes: `http-admin` turned any request error, including Ctrl-C/context cancellation, into "pass: no web admin page", and `tcpOpen` ignored the context so Ctrl-C waited out every 3 s dial. | **Fixed (in 29bd848):** dials use `DialContext`; a cancelled run reports `error`, never `pass`. Test added. |
| 3 | medium | cli | Exit code was always 0, even when the audit found a failing check, so scripts and agents could not tell "clean" from "fix needed". | **Fixed:** stable exit codes 0 (nothing to fix), 1 (error), 2 (usage), 3 (found something). Documented in `--help`. |
| 4 | medium | speed/dns | On Linux with systemd-resolved, "your current DNS" was the caching stub `127.0.0.53`, so it always looked near-instant and the DNS comparison could never recommend a faster resolver. | **Fixed:** measures the upstream servers from `/run/systemd/resolve/resolv.conf` instead. |
| 5 | medium | speed/ping | A router that blocks ping was shown as "median 0.0 ms, loss 100%", which reads like a broken link. | **Fixed:** shown as "did not answer ping (some routers block it)". |
| 6 | medium | docs/safety | README promised "it only probes your own default gateway", but the UPnP check uses SSDP multicast to the whole LAN. | **Fixed:** README states the exception precisely. |
| 7 | medium | docs/safety | PLAN.md left the door open to "default-credential checks if ever added", contradicting the no-password promise. | **Fixed:** now an explicit non-goal. |
| 8 | medium | ci | CI ran without `-race`, without staticcheck or a gofmt check, and never on Windows. | **Fixed:** CI matrix is Linux/macOS/Windows with `-race`; lint job runs gofmt and staticcheck. `make lint test` mirrors it. |
| 9 | medium | platform | Windows is not supported: gateway discovery, ping flags, Wi-Fi info and DNS discovery are macOS/Linux only. | Deferred to Phase 2 (feature work). |
| 10 | medium | tests | `internal/report` and the CLI have no tests; there is no test that a check's result maps to the right overall outcome. | Deferred to Phase 2, where the score and exit code logic get unit tests. |
| 11 | low | speed/wifi | `ParseSPAirPort` could panic on a malformed "Signal / Noise" string (`strings.Fields(...)[0]` on an empty field). | **Fixed:** parsed with `strings.Cut`, no indexing. |
| 12 | low | discover | `IsLocallyAdministered` panicked on strings shorter than 2 characters. | **Fixed (in 29bd848):** length guard. |
| 13 | low | speed/congestion | "Neighbour networks" can include the user's own router's guest SSID or mesh nodes (macOS hides BSSIDs), which inflates congestion. | Open. Suggestion threshold (2+ more than the quietest channel) limits false advice; documented in PLAN risks. |
| 14 | low | speed/bloat | The load test uses one download stream and no upload, so it can under-read fast lines and misses upload bufferbloat. | Open; Mbps is labelled approximate. Upload test listed as coming soon. |
| 15 | low | discover | A gateway in 100.64.0.0/10 (CGNAT) is accepted as "private". On a CGNAT or VPN path this may be a carrier device rather than the user's router. | Open. Only read-only, non-credential probes are sent; the gateway IP is printed first so the user can see it. |
| 16 | low | discover | `ParseProcNetRoute` ignores `Sscanf` errors, so an unparsable metric counts as 0. | Open; the kernel format is stable. |
| 17 | low | discover | `discover --online` sends the MAC prefix (first 3 bytes) to api.macvendors.com. | By design, opt-in and documented. |
| 18 | low | report | The terminal report hard-codes a "coming soon" list that duplicates `Audit.ComingSoon`. | Open (cosmetic); revisited with the report rewrite in Phase 2. |
| 19 | info | naming | Package and function names are clear; one check per file; no dead code found by staticcheck (U1000). | — |
