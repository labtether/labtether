package shared

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A connection that exists but cannot be used answered 502, shown as "An
// internal error occurred." Users must read which case it is, in fixed words
// that carry no address, token or upstream text. A failure after the request
// was sent stays hidden: a change may have run.
func TestConnectionProblemsAreSaidInFixedWords(t *testing.T) {
	dial := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")}
	setup := []struct {
		err      error
		wantCode int
		wantText string
	}{
		{fmt.Errorf("load: %w", fmt.Errorf("%w: pbs credential profile not found", ErrCollectorLoginMissing)), http.StatusConflict,
			"The saved login for the Proxmox connection is missing. Enter it again in Settings."},
		{fmt.Errorf("%w: proxmox collector config is incomplete", ErrCollectorIncomplete), http.StatusConflict,
			"The Proxmox connection has no address or login. Edit it in Settings."},
	}
	for _, tc := range setup {
		rec := httptest.NewRecorder()
		if !WriteCollectorSetupError(rec, tc.err, "Proxmox", true) || rec.Code != tc.wantCode || !strings.Contains(rec.Body.String(), tc.wantText) {
			t.Fatalf("%v: got %d %s", tc.err, rec.Code, rec.Body.String())
		}
	}
	unreachable := []error{
		fmt.Errorf("Get \"https://192.0.2.10:8006/api2/json/version\": %w", dial),
		errors.New("Get \"https://pve.example:8006/x\": dial tcp: lookup pve.example: no such host"),
		errors.New("Get \"https://192.0.2.10:8006/x\": net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)"),
		errors.New("Get \"https://192.0.2.10:8006/x\": tls: failed to verify certificate: x509: certificate signed by unknown authority"),
		errors.New("proxmox api returned 401: authentication failure"),
	}
	for _, err := range unreachable {
		rec := httptest.NewRecorder()
		ok := WriteUpstreamUnreachable(rec, err, "Proxmox")
		body := rec.Body.String()
		if !ok || rec.Code != http.StatusConflict || !strings.Contains(body, "Could not reach Proxmox. Check the connection's address and login in Settings.") {
			t.Fatalf("%v: got %v %d %s", err, ok, rec.Code, body)
		}
		if strings.Contains(body, "192.0.2.10") || strings.Contains(body, "pve.example") || strings.Contains(body, "authentication failure") {
			t.Fatalf("%v: answer leaks upstream detail: %s", err, body)
		}
	}
	hidden := []error{
		errors.New("proxmox api returned 500: VM is locked (backup)"),
		errors.New("Post \"https://192.0.2.10:8006/x\": context deadline exceeded (Client.Timeout exceeded while awaiting headers)"),
	}
	for _, err := range hidden {
		rec := httptest.NewRecorder()
		if WriteUpstreamUnreachable(rec, err, "Proxmox") || rec.Body.Len() != 0 {
			t.Fatalf("%v: must stay a hidden 502, got %s", err, rec.Body.String())
		}
	}
}
