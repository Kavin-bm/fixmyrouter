# Security policy

fixmyrouter runs on people's home networks, so we take its own safety seriously.

## Reporting a vulnerability

Please **don't open a public issue**. Report privately through GitHub: **Security → Report a vulnerability** on this repository. We aim to reply within 7 days and to fix confirmed issues before disclosing them, crediting you unless you prefer otherwise.

## What counts

Anything that breaks the safety promises in the README, for example:

- fixmyrouter sending traffic to a host other than the user's default gateway, beyond the documented public test servers (1.1.1.1, 8.8.8.8, 9.9.9.9, speed.cloudflare.com and, with `--online`, api.macvendors.com)
- sending any credential to a router, or changing any setting without the user's explicit yes and a snapshot
- a way for a website, another local user or a malicious router to drive the local UI (`fixmyrouter ui`) or the MCP server
- command injection via router responses, profile files or saved state in `~/.fixmyrouter`
- an undo that doesn't restore the snapshot

## How the local UI is protected

- It listens on `127.0.0.1` only.
- Every `/api/` request must carry a random per-process token that is only embedded in the page fixmyrouter itself serves; other websites can't read it.
- The `Host` header must be the server's own address, which blocks DNS rebinding.
- POST requests must be `application/json`, which a plain cross-site form can't send.
- A strict Content Security Policy allows only same-origin scripts, styles and requests, and all text from the router or the API is inserted as text, never HTML.

## Supported versions

Only the latest release gets security fixes until 1.0.
