# Launch drafts

Fill in the **[brackets]** with real numbers from a real before/after before posting. Don't post made-up numbers.

## X / Twitter thread

**1/**
My [parents'] Wi-Fi was "slow" for [months]. Their ISP said the line was fine. It was: their router was on the most crowded channel on the street, on 2.4 GHz, with [UPnP and Telnet] open.

So I built a tool that finds that in 10 seconds and explains it in plain English. 🧵

**2/**
`fixmyrouter check` measures what actually makes home Wi-Fi feel slow:
• band, channel and how many neighbours share it
• signal and noise
• router latency and jitter
• your DNS vs 1.1.1.1 / 8.8.8.8 / 9.9.9.9
• lag under load (bufferbloat)
…and gives you one score out of 100.

**3/**
Before: **[score]**/100 [screenshot]
After switching to channel [N], moving to 5 GHz and turning on QoS: **[score]**/100 [screenshot]
Video calls went from [stuttering] to [fine].

**4/**
It's paranoid on purpose:
• read-only until you say yes
• never logs in to your router or tries a password
• only talks to your own router
• every change is snapshotted, and `fixmyrouter undo` reverts it
• no account, no telemetry, one Go binary

**5/**
The part I like most: paste one prompt into Claude Code / Codex / Gemini CLI and your agent runs it, explains it to your mum, and asks before every change. There's an MCP server too.

**6/**
Router admin pages are all different, so fixmyrouter grows through tiny YAML profiles: "on a TP-Link Archer C6, UPnP lives under Advanced > NAT Forwarding". If you own a JioFiber, Airtel Xstream, TP-Link, D-Link or Netgear router, a profile PR takes 5 minutes 🙏

github.com/kavinbm/fixmyrouter

## Show HN

**Title:** Show HN: Fixmyrouter – find out why your home Wi-Fi is slow or unsafe (read-only, one binary)

**Body:**

I kept getting "the internet is slow" calls from family. The line was fine every time; the problem was in the house: a crowded 2.4 GHz channel, a weak signal two walls away, an ISP router with UPnP and Telnet on, or a slow ISP resolver.

fixmyrouter is a single Go binary that checks those things and gives a 0-100 score with plain-English fixes:

- Wi-Fi band, channel, width, signal/noise and the neighbour networks your OS already scanned (macOS `system_profiler`, Linux `iw`/`nmcli`, Windows `netsh`)
- router latency and jitter, DNS comparison, and an opt-in bufferbloat test
- router security: plain-HTTP admin page, Telnet, UPnP IGD, Wi-Fi encryption

Design choices HN might care about:

- Read-only by default. The only automatic fix today is switching your computer's DNS (macOS), with a snapshot, a verification read-back and `undo`. It never logs in to the router or sends a credential; router changes are instructions, with exact menu paths from community YAML profiles.
- It only probes your own default gateway (from the routing table, so there's no target argument), plus the public resolvers and the Cloudflare download you'd expect from a speed check.
- `fixmyrouter ui` serves a local dashboard on 127.0.0.1 with go:embed: no external requests, a per-run token and a Host check against DNS rebinding.
- `--json`, stable exit codes and an MCP server, so coding agents can run it for non-technical people and ask before each change (PROMPT.md).

What I'd love feedback on: the thresholds (when is jitter "bad"?), whether the channel advice holds up in dense apartments, and Linux/Windows reports. Only macOS has been run end to end so far.

[before/after screenshot link]
