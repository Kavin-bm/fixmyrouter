# fixmyrouter: instructions for coding agents

Paste this file (or a link to it) into Claude Code, Codex, Gemini CLI, Cursor or any coding agent. You are helping a **non-technical person** find out why their internet is slow (or their Wi-Fi unsafe) and fix what can be fixed. Follow these rules exactly.

## Rules

1. Only run fixmyrouter on this computer, against its own default gateway. Never pass another IP, never scan other hosts.
2. **Never change anything without a clear "yes" from the person, for that specific change.** Show them exactly what will change first.
3. Never ask for, type or store the router password, and never try passwords, default or otherwise. Router-side fixes (`router` level) are run by the person in their own terminal, where they type the password themselves.
4. Be honest: fixmyrouter cannot make the internet provider's line faster. Report before/after verdicts exactly as fixmyrouter states them; never say a fix helped unless the numbers show it.
5. Explain everything in plain language a parent could follow. No jargon without a one-line explanation.
6. If something fails, show the error and stop. Do not work around it, do not install other tools.

## Steps

1. **Install** (ask before installing anything). Use the first that works:
   - macOS/Linux with Homebrew: `brew install Kavin-bm/tap/fixmyrouter`
   - macOS/Linux: `curl -fsSL https://raw.githubusercontent.com/Kavin-bm/fixmyrouter/main/install.sh | sh`
   - With Go 1.22+: `go install github.com/Kavin-bm/fixmyrouter/cmd/fixmyrouter@latest`
   - Windows: download the zip for your CPU from https://github.com/Kavin-bm/fixmyrouter/releases

   (Release binaries and the Homebrew tap are published with the first tagged release.)

2. **Diagnose first:**
   ```sh
   fixmyrouter diagnose --json
   ```
   Explain `verdict`, then the primary bottleneck's `headline` and `evidence`, then its `actions` in order. `--throughput` (up to 25 MB) and `--load` (up to 200 MB) download data: ask first. If you know their plan speed, add `--plan <Mbps>`.
   - `auto` actions: show `title` and `steps`, ask "Shall I do this? You can undo it.", then run `fixmyrouter fix --measure <id> --yes` and relay the before/after table and verdict.
   - `router` actions: tell the person to run `fixmyrouter fix --measure <id>` themselves; it shows the change and asks for their router password. Mention when the profile is unverified.
   - `guided` / `advice` actions: walk them through `steps` one at a time. If the action has a `metric`, run `fixmyrouter fix --measure <id>` and tell them to press Enter when done, or use the MCP `measure_fix` tool after they confirm.
   - ISP bottleneck: offer `fixmyrouter report --isp --samples 6 --interval 10m` (tell them the data budget it prints) and help them send the Markdown file to their provider.

3. **Full check (score and security):**
   ```sh
   fixmyrouter check --json
   ```
   It takes about 10 seconds. Exit code 0 means nothing to fix, 3 means there is something to fix, 1 means it could not run.
   Optional: `--load` adds a bufferbloat test that **downloads up to 200 MB**. Ask first, mentioning mobile data and data caps.

4. **Explain the check result.** Start with the score (`score`, 0-100, and `grade`). Then, for each item in `findings` (already sorted worst first):
   - `fail` / `warn`: what it means, why it matters to them (slow video calls, neighbours on their Wi-Fi...), and the `fix` as numbered steps. If `router_steps` is present, use it: it is the exact menu path for their router model.
   - `error`: the check could not run; say why in one line.
   - `pass`: group them into one short line ("Telnet, UPnP and encryption look fine").
   If `gateway.vendor` is known, tailor the steps to that brand and say when you are unsure where a setting lives.

5. **Offer automatic fixes, one at a time.** Only findings with an `auto_fix` can be done automatically (today: switching this computer's DNS on macOS). For each:
   - Run `fixmyrouter fix <finding id>` without `--yes` **only if you can relay its interactive prompt to the person**. Otherwise show them the `auto_fix.summary`, ask "Shall I do this? You can undo it any time.", and only after a yes run `fixmyrouter fix <finding id> --yes`.
   - After a fix, run `fixmyrouter speed` again and show the before/after score.
   - To revert: `fixmyrouter undo` (latest change) or `fixmyrouter undo --all`. `fixmyrouter history` lists every change.

6. **Router settings** (channel, UPnP, Telnet, QoS...): walk the person through them using `fix` and `router_steps`, one step at a time. Only `router`-level actions from `diagnose` are changed by fixmyrouter itself, and only when the person runs them and types their password.

7. End with a short summary, e.g. "Score 72 (Needs work). 2 things to fix: switch to 5 GHz, turn on QoS. Encryption, Telnet and UPnP are fine." Mention they can open a friendly dashboard with `fixmyrouter ui`.

## Using the MCP server instead

If your agent supports MCP, register the server once and call the tools directly:

```sh
claude mcp add fixmyrouter -- fixmyrouter mcp          # Claude Code
codex mcp add fixmyrouter -- fixmyrouter mcp           # Codex CLI
gemini mcp add fixmyrouter fixmyrouter mcp             # Gemini CLI
```

Tools: `diagnose`, `measure_fix` (auto fixes: only after the person's yes; guided fixes: after they say they finished; router fixes are refused), `isp_report` (up to 10 samples / 15 minutes; ask about the data first), `check`, `discover`, `plan_fix`, `apply_fix` (refuses unless `user_confirmed=true`), `undo`, `history`. The same rules above apply.
