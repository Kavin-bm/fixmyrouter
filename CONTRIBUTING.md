# Contributing to fixmyrouter

Thanks for helping make home Wi-Fi faster and safer!

## Easiest way to help: add your router (no Go needed)

1. Run `fixmyrouter discover --json` and note `http_title` and `http_server`.
2. Copy `profiles/tp-link/archer-c6-v3.yaml` to `profiles/<vendor>/<model>.yaml` and fill in the menu paths from your own router's admin page.
3. Run `fixmyrouter profiles validate profiles` (or `go run ./cmd/fixmyrouter profiles validate profiles`).
4. Open a pull request titled `profile: <vendor> <model>`.

Details and rules: [profiles/README.md](profiles/README.md). No PR? Use the **Add my router** issue form.

## Second easiest: share a before/after result

Run `fixmyrouter check`, change something, run it again, and open a **Share a before/after result** issue. Real numbers are how we tune the thresholds.

## Code

- Go 1.22+. `make lint test` runs gofmt, go vet, staticcheck and `go test -race`, the same as CI.
- `make build && ./bin/fixmyrouter ui` to see your change in the UI. The frontend is plain HTML/CSS/JS in `internal/ui/static/`, with no build step.
- Layout: `internal/checks` (security checks, one per file), `internal/speed` (measurements), `internal/run` (shared by CLI, UI and MCP), `internal/fix` (snapshot, apply, undo), `internal/report`, `internal/ui`, `internal/mcp`, `profiles/`.
- OS-specific output is parsed by pure functions with a unit test that uses real sample output. Paste real output from your machine into the test.
- Keep it small: standard library first, one dependency today (yaml.v3). Don't add a dependency for something a few lines can do.

### Non-negotiable safety rules

PRs that break these are closed:

- Checks are **read-only**. Anything that changes something must go through `internal/fix`: a plan shown to the user, an explicit yes, a snapshot saved first, and an undo.
- Talk **only to the default gateway** (plus documented public test servers in `speed`). Never accept an arbitrary target IP.
- **Never send credentials** except the password a person types for a profile-allowed router fix (`internal/routerfix`: once, to the gateway, never stored); never guess passwords, no exploits, no scanning other hosts.
- No telemetry, accounts or cloud services. The UI loads nothing from the internet.
- Every finding needs a `Detail` a non-technical person understands and a `Fix`.
- Don't claim something works in docs unless you ran it. Mark the rest "coming soon".

## Reporting bugs

Open a **Bug report** with your OS, `fixmyrouter version`, and the output of `fixmyrouter check --json` (remove your MAC address if you prefer). Security issues: see [SECURITY.md](SECURITY.md).

By contributing you agree to the [code of conduct](CODE_OF_CONDUCT.md) and to license your work under MIT.
