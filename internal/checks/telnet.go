package checks

import (
	"context"
	"net"
)

// Telnet checks whether TCP port 23 is open on the gateway. It only opens and
// closes a connection; it never reads a login prompt or sends anything.
func Telnet(ctx context.Context, ip string) Finding {
	return telnet(ctx, net.JoinHostPort(ip, "23"))
}

func telnet(ctx context.Context, addr string) Finding {
	f := Finding{ID: "telnet", Title: "Telnet (port 23)"}
	open := tcpOpen(ctx, addr)
	if ctx.Err() != nil {
		return errFinding(f, ctx.Err())
	}
	if !open {
		f.Status, f.Severity = StatusPass, "info"
		f.Detail = "Port 23 is closed or filtered."
		return f
	}
	f.Status, f.Severity = StatusFail, "high"
	f.Detail = "Telnet is open. It sends everything, including passwords, in clear text and is a favourite target of IoT botnets like Mirai."
	f.Fix = "Disable Telnet in the router admin page (often under System Tools / Administration / Remote Access). If there is no option, ask your ISP or check for a firmware update."
	return f
}
