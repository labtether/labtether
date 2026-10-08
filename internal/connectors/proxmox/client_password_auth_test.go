package proxmox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientPasswordAuthTicketCaching(t *testing.T) {
	allowInsecureTransportForProxmoxTests(t)
	var ticketCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/access/ticket":
			ticketCalls.Add(1)
			_, _ = w.Write([]byte(`{"data":{"ticket":"PVE:ticket-1","CSRFPreventionToken":"csrf-1"}}`))
		case "/api2/json/version":
			if cookie := r.Header.Get("Cookie"); !strings.Contains(cookie, "PVEAuthCookie=PVE:ticket-1") {
				t.Fatalf("expected auth cookie on GET, got %q", cookie)
			}
			_, _ = w.Write([]byte(`{"data":{"release":"8.3"}}`))
		case "/api2/json/nodes/pve01/qemu/100/status/stop":
			if cookie := r.Header.Get("Cookie"); !strings.Contains(cookie, "PVEAuthCookie=PVE:ticket-1") {
				t.Fatalf("expected auth cookie on POST, got %q", cookie)
			}
			if csrf := r.Header.Get("CSRFPreventionToken"); csrf != "csrf-1" {
				t.Fatalf("expected CSRFPreventionToken=csrf-1, got %q", csrf)
			}
			_, _ = w.Write([]byte(`{"data":"UPID:stop"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL:  server.URL,
		AuthMode: AuthModePassword,
		Username: "root@pam",
		Password: "secret",
		Timeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if !client.IsConfigured() {
		t.Fatalf("expected password-mode client to be configured")
	}
	if client.GetAuthMode() != AuthModePassword {
		t.Fatalf("expected auth mode password, got %s", client.GetAuthMode())
	}

	release, err := client.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}
	if release != "8.3" {
		t.Fatalf("unexpected release: %s", release)
	}
	upid, err := client.StopVM(context.Background(), "pve01", "100")
	if err != nil {
		t.Fatalf("StopVM failed: %v", err)
	}
	if upid != "UPID:stop" {
		t.Fatalf("unexpected upid: %s", upid)
	}
	if ticketCalls.Load() != 1 {
		t.Fatalf("expected ticket endpoint to be called once, got %d", ticketCalls.Load())
	}
}
