# fixmyrouter: Goal

## One-line pitch
Your Wi-Fi is slower than it should be. Your coding agent finds out why and walks you through the fix. Security checks come along for free. Safe by default, with a report and (soon) a one-command undo.

## The problem
Home Wi-Fi is slow for boring, fixable reasons: the router sits on the same crowded channel as ten neighbours, devices cling to 2.4 GHz, the signal is weak in the room where people work, the ISP's DNS is slow, and the line lags whenever someone downloads (bufferbloat). Nobody measures this; people just pay for a faster plan. The same routers also ship with risky settings (UPnP on, Telnet open, admin page over HTTP) that nobody checks, and the admin pages are different on every model. Parents and non-technical users never touch them.

## Who it is for
- Developers who set up Wi-Fi for family and friends.
- Anyone who can paste one command or prompt into a coding agent (Claude Code, Codex, Gemini CLI).
- Starting market: India (Jio, Airtel, TP-Link, D-Link, Netgear, ISP-supplied routers, dense apartment buildings with crowded channels), but the tool is global.

## What success looks like
1. A user runs one command and gets a plain-language speed report with concrete suggestions in under a minute.
2. A before/after run shows a measurable win (lower router ping/jitter, faster DNS, better bufferbloat grade).
3. Security is the second report (`audit`), not the headline.
4. The tool fixes only what the user approves, and every change is logged and reversible (coming with `fix`/`undo`).
5. Contributors add router model profiles (a YAML file per model) without writing Go.

## Principles (the cleanyourtv recipe)
- Solve one clear pain (slow Wi-Fi) with a tiny core.
- Trust promises: read-only by default, never changes anything without a yes, one command undoes it all.
- A prompt file (`PROMPT.md`) lets any coding agent drive the tool: "paste this into your agent".
- Per-model data in `profiles/`, so community PRs scale coverage.
- Single static Go binary, no dependencies. Heavy tests (like the bufferbloat download) are opt-in.

## Non-goals
- No exploitation, brute forcing or attacking other people's networks. It only talks to the user's own router.
- No cloud service, telemetry or account.
- Not a full vulnerability scanner, and not a replacement for an ISP speed test.
