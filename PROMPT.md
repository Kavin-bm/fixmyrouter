# fixmyrouter: instructions for coding agents

Paste this file (or a link to it) into Claude Code, Codex, Gemini CLI, Cursor or any coding agent. You are helping a **non-technical person** find out why their home Wi-Fi is slow or unsafe. Follow these rules exactly.

## Rules

1. Only run fixmyrouter on this computer, against its own default gateway. Never pass another IP, never scan other hosts.
2. **Never change anything without a clear "yes" from the person, for that specific change.** Show them exactly what will change first.
3. Never log in to the router and never try passwords, default or otherwise. fixmyrouter does not do this and you must not either.
4. Explain everything in plain language a parent could follow. No jargon without a one-line explanation.
5. If something fails, show the error and stop. Do not work around it, do not install other tools.

## Steps

1. **Install** (ask before installing anything). Use the first that works:
   - macOS/Linux with Homebrew: `brew install kavinbm/tap/fixmyrouter`
   - macOS/Linux: `curl -fsSL https://raw.githubusercontent.com/kavinbm/fixmyrouter/main/install.sh | sh`
   - With Go 1.22+: `go install github.com/kavinbm/fixmyrouter/cmd/fixmyrouter@latest`
   - Windows: download the zip for your CPU from https://github.com/kavinbm/fixmyrouter/releases

   (Release binaries and the Homebrew tap are published with the first tagged release.)

2. **Run the check:**
   ```sh
   fixmyrouter check --json
   ```
   It takes about 10 seconds. Exit code 0 means nothing to fix, 3 means there is something to fix, 1 means it could not run.
   Optional: `--load` adds a bufferbloat test that **downloads up to 200 MB**. Ask first, mentioning mobile data and data caps.

3. **Explain the result.** Start with the score (`score`, 0-100, and `grade`). Then, for each item in `findings` (already sorted worst first):
   - `fail` / `warn`: what it means, why it matters to them (slow video calls, neighbours on their Wi-Fi...), and the `fix` as numbered steps. If `router_steps` is present, use it: it is the exact menu path for their router model.
   - `error`: the check could not run; say why in one line.
   - `pass`: group them into one short line ("Telnet, UPnP and encryption look fine").
   If `gateway.vendor` is known, tailor the steps to that brand and say when you are unsure where a setting lives.

4. **Offer automatic fixes, one at a time.** Only findings with an `auto_fix` can be done automatically (today: switching this computer's DNS on macOS). For each:
   - Run `fixmyrouter fix <finding id>` without `--yes` **only if you can relay its interactive prompt to the person**. Otherwise show them the `auto_fix.summary`, ask "Shall I do this? You can undo it any time.", and only after a yes run `fixmyrouter fix <finding id> --yes`.
   - After a fix, run `fixmyrouter speed` again and show the before/after score.
   - To revert: `fixmyrouter undo` (latest change) or `fixmyrouter undo --all`. `fixmyrouter history` lists every change.

5. **Router settings** (channel, UPnP, Telnet, QoS...) are never changed by fixmyrouter. Walk the person through them using `fix` and `router_steps`, one step at a time, and ask them to tell you when each step is done.

6. End with a short summary, e.g. "Score 72 (Needs work). 2 things to fix: switch to 5 GHz, turn on QoS. Encryption, Telnet and UPnP are fine." Mention they can open a friendly dashboard with `fixmyrouter ui`.

## Using the MCP server instead

If your agent supports MCP, register the server once and call the tools directly:

```sh
claude mcp add fixmyrouter -- fixmyrouter mcp          # Claude Code
codex mcp add fixmyrouter -- fixmyrouter mcp           # Codex CLI
gemini mcp add fixmyrouter fixmyrouter mcp             # Gemini CLI
```

Tools: `check`, `discover`, `plan_fix`, `apply_fix` (refuses unless `user_confirmed=true`; only set it after the person said yes to the `plan_fix` result), `undo`, `history`. The same rules above apply.
