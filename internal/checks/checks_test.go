package checks

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPAdminPlain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<title>Login</title>"))
	}))
	defer srv.Close()
	f := httpAdmin(context.Background(), srv.URL+"/", "127.0.0.1:1")
	if f.Status != StatusWarn {
		t.Fatalf("want warn, got %+v", f)
	}
}

func TestHTTPAdminRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://192.168.1.1/", http.StatusFound)
	}))
	defer srv.Close()
	f := httpAdmin(context.Background(), srv.URL+"/", "127.0.0.1:1")
	if f.Status != StatusPass {
		t.Fatalf("want pass, got %+v", f)
	}
}

func TestCancelledIsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if f := httpAdmin(ctx, "http://127.0.0.1:1/", "127.0.0.1:1"); f.Status != StatusError {
		t.Fatalf("http-admin after cancel: %+v", f)
	}
	if f := telnet(ctx, "127.0.0.1:1"); f.Status != StatusError {
		t.Fatalf("telnet after cancel: %+v", f)
	}
}

func TestTelnet(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if f := telnet(context.Background(), ln.Addr().String()); f.Status != StatusFail {
		t.Fatalf("open port: %+v", f)
	}
	if f := telnet(context.Background(), "127.0.0.1:1"); f.Status != StatusPass {
		t.Fatalf("closed port: %+v", f)
	}
}

func TestSSDPHeader(t *testing.T) {
	msg := "HTTP/1.1 200 OK\r\nSERVER: Linux/3.10 UPnP/1.0 MiniUPnPd/2.1\r\nST: upnp:rootdevice\r\n\r\n"
	if got := ssdpHeader(msg, "server"); got != "Linux/3.10 UPnP/1.0 MiniUPnPd/2.1" {
		t.Fatalf("got %q", got)
	}
}

func TestUPnPFinding(t *testing.T) {
	root := "HTTP/1.1 200 OK\r\nST: upnp:rootdevice\r\nUSN: uuid:1::upnp:rootdevice\r\n\r\n"
	igd := "HTTP/1.1 200 OK\r\nST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\n\r\n"
	if f := upnpFinding(Finding{}, nil); f.Status != StatusPass {
		t.Fatalf("no replies: %+v", f)
	}
	if f := upnpFinding(Finding{}, []string{root}); f.Status != StatusPass {
		t.Fatalf("media server only: %+v", f)
	}
	if f := upnpFinding(Finding{}, []string{root, igd}); f.Status != StatusWarn {
		t.Fatalf("igd: %+v", f)
	}
}

func TestScore(t *testing.T) {
	fs := []Finding{
		{Status: StatusFail, Severity: "high"},
		{Status: StatusWarn, Severity: "medium"},
		{Status: StatusPass, Severity: "info"},
		{Status: StatusError, Severity: "high"},
	}
	if got := Score(fs); got != 63 {
		t.Fatalf("score %d", got)
	}
	if Score(nil) != 100 || Grade(100) != "Great" || Grade(63) != "Needs work" {
		t.Fatal("empty score or grade")
	}
	many := make([]Finding, 10)
	for i := range many {
		many[i] = Finding{Status: StatusFail, Severity: "high"}
	}
	if Score(many) != 0 {
		t.Fatal("score must not go below 0")
	}
}

func TestWiFiSecurity(t *testing.T) {
	for mode, want := range map[string]string{
		SecOpen: StatusFail, SecWEP: StatusFail, SecWPA: StatusWarn, SecWPAWPA2: StatusWarn,
		SecWPA2: StatusPass, SecWPA3: StatusPass, "": StatusError,
	} {
		if f := WiFiSecurity(mode); f.Status != want {
			t.Errorf("%q: got %s", mode, f.Status)
		}
	}
}
