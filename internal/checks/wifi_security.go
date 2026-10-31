package checks

// Normalised Wi-Fi security modes, as reported by the host OS.
const (
	SecOpen       = "open"
	SecWEP        = "wep"
	SecWPA        = "wpa"
	SecWPAWPA2    = "wpa-wpa2"
	SecWPA2       = "wpa2"
	SecWPA2WPA3   = "wpa2-wpa3"
	SecWPA3       = "wpa3"
	SecEnterprise = "enterprise"
)

// WiFiSecurity grades the security mode of the Wi-Fi network this computer
// is on. It reads what the OS already knows; nothing is sent.
func WiFiSecurity(mode string) Finding {
	f := Finding{ID: "wifi-security", Title: "Wi-Fi encryption"}
	setup := "In the router admin page, under Wireless / Wireless Security, choose WPA2-Personal (AES) or WPA3-Personal and set a long password. Then reconnect your devices."
	switch mode {
	case SecOpen:
		f.Status, f.Severity, f.Fix = StatusFail, "high", setup
		f.Detail = "Your Wi-Fi has no password or encryption. Anyone nearby can join it and see unencrypted traffic."
	case SecWEP:
		f.Status, f.Severity, f.Fix = StatusFail, "high", setup
		f.Detail = "Your Wi-Fi uses WEP, which can be cracked in minutes with free tools."
	case SecWPA:
		f.Status, f.Severity, f.Fix = StatusWarn, "medium", setup
		f.Detail = "Your Wi-Fi uses the original WPA (TKIP), which is outdated and also slows Wi-Fi down to old 54 Mbps speeds."
	case SecWPAWPA2:
		f.Status, f.Severity = StatusWarn, "low"
		f.Detail = "Your Wi-Fi allows old WPA (TKIP) alongside WPA2. Devices that fall back to it are less secure and slower."
		f.Fix = "In the router's wireless security settings, choose 'WPA2-PSK' with 'AES' only (not 'WPA/WPA2' or 'TKIP+AES')."
	case SecWPA2, SecWPA2WPA3, SecWPA3, SecEnterprise:
		f.Status, f.Severity = StatusPass, "info"
		f.Detail = "Your Wi-Fi uses " + map[string]string{SecWPA2: "WPA2", SecWPA2WPA3: "WPA2/WPA3", SecWPA3: "WPA3", SecEnterprise: "WPA2/WPA3 Enterprise"}[mode] + ", which is fine."
	default:
		f.Status, f.Severity = StatusError, "info"
		f.Detail = "The OS did not report the Wi-Fi security mode."
	}
	return f
}
