# fixmyrouter: instructions for coding agents

You are helping a non-technical person make their home Wi-Fi faster (and safer). Follow these rules exactly.

## Rules

1. Only run fixmyrouter against the user's **own** network. Never pass another IP, never scan other hosts.
2. **Do not change any router setting yourself.** Do not log in to the router. Do not try passwords, default or otherwise. Give the user steps to follow instead.
3. Do not run `speed --load` without asking first: it downloads up to 200 MB (matters on mobile data or hotspots).
4. Explain in plain language a parent could follow. No jargon without a one-line explanation.
5. If something fails, show the error and stop. Do not work around it.

## Steps

1. Check Go is installed: `go version`. If missing, tell the user and stop (don't install software without asking).
2. Run the speed check:
   ```sh
   go run github.com/kavinbm/fixmyrouter/cmd/fixmyrouter@latest speed --json
   ```
   (TODO: replace with the release binary or Homebrew command once published.)
3. Explain the results:
   - `wifi`: band (2.4 GHz vs 5 GHz), channel, and `signal_dbm` (above -60 excellent, -67 good, -75 fair, below that weak).
   - `congestion`: how many neighbours share the channel, and the quietest channel.
   - `gateway_latency`: healthy Wi-Fi is under ~5 ms median with low jitter.
   - `dns`: compare the `system` resolver with the public ones.
   - `bufferbloat`: only present if `--load` was used.
4. Walk through each item in `suggestions` in order as numbered steps. If the router brand is known (`fixmyrouter discover --json`), tailor the menu paths and say when you're unsure.
5. Offer the security check too: run `fixmyrouter audit --json` and explain each finding (`pass` in one line; `warn`/`fail` with the `fix` text as steps).
6. Ask the user to run `speed` again after changes to compare before/after.
7. Mention that automatic fixes and undo are **coming soon**, not available yet.
