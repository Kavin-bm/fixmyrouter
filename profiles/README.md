# Router profiles

A profile tells fixmyrouter where each speed and security setting lives on one router model (channel, band steering, DNS, QoS, UPnP...). When `fixmyrouter check` recognises your router from its admin page, every finding gets a `router_steps` line with the exact menu path, in the terminal, the UI and the JSON for agents. Profiles are embedded in the binary, so they work offline.

Profiles are mainly **instructions for a person**. A profile may also allow a few router-side changes with a `router_fix` block (see below); without one, fixmyrouter never logs in to that router.

## `router_fix` (automatic router changes)

```yaml
router_fix:
  driver: openwrt-ubus                       # a driver built into fixmyrouter
  settings: [wifi_channel_2g, wifi_channel_5g, sqm_qos]
```

- Only settings listed here can be changed by `fixmyrouter router apply` / `fix --measure`, and only settings the driver supports (the validator checks both).
- The person types the admin password; it is sent once to the gateway, never stored and never retried. Profiles must never contain credentials.
- Add a driver only for a documented local API, with tests against a fake router (see `internal/routerfix`). Keep `verified: false` until someone has applied **and undone** each setting on a real device.
- Today: `openwrt-ubus` (OpenWrt LuCI profile, **unverified**).

## Add yours in 5 minutes

1. Run `fixmyrouter discover --json` and note `http_title` and `http_server`.
2. Copy [`tp-link/archer-c6-v3.yaml`](tp-link/archer-c6-v3.yaml) to `profiles/<vendor>/<model>.yaml` (lowercase, dashes).
3. Fill in the menu paths from your own router's admin page. Use `"n/a"` when an option does not exist.
4. Set `verified: true` only if you clicked through every path on a real device.
5. Run `fixmyrouter profiles validate profiles` (or `go run ./cmd/fixmyrouter profiles validate profiles`) and open a PR titled `profile: <vendor> <model>`.

No Go needed. Editors that understand JSON Schema (VS Code with the YAML extension) autocomplete and check the file via the `# yaml-language-server: $schema=../schema.json` line. Can't make a PR? Use the "Add my router" issue form instead.

## Rules

- Only describe settings and menu paths. **Never include passwords, default credentials, exploits or anything that bypasses login.** The validator rejects anything that looks like a username or password.
- Say which firmware version you checked, and where the paths come from (`source`).
- `verified: false` profiles are labelled as unverified in guided steps and router fixes.
- Allowed settings: `wifi_channel_2g`, `wifi_channel_5g`, `band_steering`, `dns_servers`, `sqm_qos`, `telnet`, `upnp`, `https_admin`, `wps`, `remote_management`, `wifi_security`.

## Template

```yaml
# yaml-language-server: $schema=../schema.json
vendor: TP-Link              # brand as printed on the device
model: Archer C6             # exact model name
hardware_version: v3         # from the sticker, if any
firmware_tested: "1.3.6 Build 20200902"
isp_branded: false           # true for ISP routers (JioFiber, Airtel Xstream, ...)
contributed_by: your-github-username
verified: true               # you checked every path on a real device
source: "my own router"

# How to recognise this router from `fixmyrouter discover --json`
match:
  http_title_contains: ["Archer C6"]
  http_server_contains: []

admin:
  url: http://192.168.0.1/
  https_supported: true

# Where each setting lives in the admin UI. Use "n/a" if the router has no such option.
settings:
  # speed
  wifi_channel_2g:
    menu_path: "Advanced > Wireless > Wireless Settings > 2.4GHz > Channel"
    notes: "Prefer 1, 6 or 11."
  wifi_channel_5g:
    menu_path: "Advanced > Wireless > Wireless Settings > 5GHz > Channel"
    notes: ""
  band_steering:
    menu_path: "n/a"
    notes: ""
  dns_servers:
    menu_path: "Advanced > Network > DHCP Server > Primary/Secondary DNS"
    notes: ""
  sqm_qos:
    menu_path: "Advanced > QoS"
    notes: "Set bandwidth slightly below your plan speed."
  # security
  telnet:
    menu_path: "n/a"
    notes: ""
  upnp:
    menu_path: "Advanced > NAT Forwarding > UPnP"
    notes: "Turn off unless a game console needs it."
  https_admin:
    menu_path: "Advanced > System > Administration > Local Management"
    notes: ""
  wps:
    menu_path: "Advanced > Wireless > WPS"
    notes: ""
  remote_management:
    menu_path: "Advanced > System > Administration > Remote Management"
    notes: ""
```
