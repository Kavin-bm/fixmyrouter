## What and why

<!-- One or two sentences. For a profile: "profile: TP-Link Archer C6 v3". -->

## Checklist

- [ ] `make lint test` passes (or, for a profile only: `fixmyrouter profiles validate profiles`)
- [ ] Read-only by default: nothing changes without an explicit yes and a snapshot
- [ ] Talks only to the user's own default gateway (or a documented public test server)
- [ ] No credentials, no password guessing, no exploits
- [ ] New output is plain English, with a "how to fix"
- [ ] I ran it on my own machine (say which OS) and saw it work
