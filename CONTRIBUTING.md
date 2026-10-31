# Contributing to fixmyrouter

Thanks for helping make home Wi-Fi faster and safer!

## Easiest way to help: add your router

1. Run `fixmyrouter discover --json` and note `http_title` and `http_server`.
2. Copy the template from [profiles/README.md](profiles/README.md) to `profiles/<vendor>/<model>.yaml`.
3. Fill in the menu paths from your own router's admin page.
4. Open a pull request titled `profile: <vendor> <model>`.

## Code contributions

- Go 1.22+. Run `make lint test` before opening a PR.
- Security checks: one per file in `internal/checks/`. Speed measurements: `internal/speed/`. Every parser and suggestion rule needs a unit test.
- Every check must be **read-only** (heavy tests like `--load` must stay opt-in), talk **only to the default gateway**, and **never send credentials**. PRs that add brute forcing, default-password attempts, exploits or scanning of other hosts will be closed.
- Keep output plain-English: every finding needs a `Detail` a non-technical person understands and a `Fix`.

## Reporting bugs

Open an issue with your OS, `fixmyrouter version`, and the output of `fixmyrouter speed --json` or `audit --json` (remove your MAC address if you prefer).
