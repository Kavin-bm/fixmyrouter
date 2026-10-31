# Router profiles

A profile tells fixmyrouter where each speed and security setting lives on one router model (channel, band steering, DNS, QoS, UPnP...), so future versions can give exact steps (and, later, apply fixes with your consent). **Profiles are not used by the code yet; the loader is coming soon.** Adding them now helps us launch with wide coverage.

## Rules

- One file per model: `profiles/<vendor>/<model>.yaml`, lowercase, dashes (e.g. `profiles/tp-link/archer-c6-v3.yaml`).
- Only describe settings and menu paths. **Never include passwords, default credentials, exploits or anything that bypasses login.**
- Say which firmware version you checked, and test the menu paths on your own router.

## Template

```yaml
# profiles/<vendor>/<model>.yaml
vendor: TP-Link              # brand as printed on the device
model: Archer C6             # exact model name
hardware_version: v3         # from the sticker, if any
firmware_tested: "1.3.6 Build 20200902"
isp_branded: false           # true for ISP routers (JioFiber, Airtel Xstream, ...)
contributed_by: your-github-username

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
