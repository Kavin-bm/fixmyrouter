# Good first issues

Each one is sized for a first PR. Copy the title and body into a GitHub issue and label it `good first issue`.

---

### 1. Add a profile for your router
**Labels:** router profile, good first issue
Copy `profiles/tp-link/archer-c6-v3.yaml`, fill in your router's menu paths, run `fixmyrouter profiles validate profiles`. No Go needed. Wanted first: JioFiber, Airtel Xstream, TP-Link Archer C6/C50/C80, D-Link DIR-615/825, Netgear R6xxx, ASUS RT-AX series.

### 2. Verify the TP-Link Archer C6 v3 profile on a real device
**Labels:** router profile, good first issue
The only built-in profile comes from the vendor manual and is marked `verified: false`. If you own one, click through each `menu_path`, fix what's wrong, and set `verified: true`.

### 3. Run fixmyrouter on Linux and report back
**Labels:** linux, help wanted
Parsers for `iw`, `nmcli`, `/proc/net/route` and `/etc/resolv.conf` have unit tests but haven't run on real hardware. Run `fixmyrouter check --json` on your laptop and open a "Share a result" issue, or a PR adding your real `iw dev <if> link` output as a test case.

### 4. Run fixmyrouter on Windows and report back
**Labels:** windows, help wanted
Same as above for `route print`, `arp -a`, `netsh wlan show interfaces` and `netsh wlan show networks mode=bssid`. Non-English Windows is especially useful: the `netsh` parser expects English labels today.

### 5. DNS fix on Linux (NetworkManager)
**Labels:** linux, enhancement
`internal/fix` only supports macOS (`networksetup`). Add `nmcli con mod <conn> ipv4.dns ... ipv4.ignore-auto-dns yes` with the same snapshot/verify/undo flow, and fake the commands in tests like `fix_test.go` does.

### 6. Upload bufferbloat
**Labels:** speed, enhancement
`internal/speed/bloat.go` only loads the downlink. Add an upload phase (POST to `https://speed.cloudflare.com/__up`) behind the same `--load` flag and cap, and report both grades.

### 7. WPS check from the OS scan
**Labels:** security, enhancement
Some OS scans expose WPS (e.g. `iw dev <if> scan` shows a "WPS:" block). Add a read-only `wps` finding when the user's own network advertises WPS, with a test from real output.

### 8. "Save as image" for the score card
**Labels:** ui, enhancement
Add a button in `internal/ui/static/app.js` that renders the score and before/after to a `<canvas>` and downloads a PNG, with no libraries and no network calls.

### 9. Localise netsh labels
**Labels:** windows, i18n
`ParseNetshInterfaces` matches English keys ("Channel", "Signal"). Add German/Spanish/Hindi key aliases with real sample output in tests.

### 10. Explain each score deduction in the UI
**Labels:** ui, good first issue
Show "-12" next to each medium finding and "-25" next to each high one, so people can see where the points went. The weights are in `checks.Score`.
