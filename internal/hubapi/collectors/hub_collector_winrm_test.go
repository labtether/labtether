package collectors_test

import (
	"context"
	collectorspkg "github.com/labtether/labtether/internal/hubapi/collectors"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecuteWinRMCommandRejectsInvalidCAPEM(t *testing.T) {
	_, err := collectorspkg.ExecuteWinRMCommand(
		context.Background(),
		"https://127.0.0.1:5986/wsman",
		"Administrator",
		"password",
		"Get-Date",
		true,
		false,
		"not-a-pem-certificate",
	)
	if err == nil {
		t.Fatalf("expected error for invalid ca_pem")
	}
	if !strings.Contains(err.Error(), "invalid ca_pem") {
		t.Fatalf("expected invalid ca_pem error, got: %v", err)
	}
}

func TestWinRMSOAPRequestRejectsOversizedResponse(t *testing.T) {
	testutil.AllowInsecureTransportForConnectorTests(t)

	oversizedBody := strings.Repeat("A", collectorspkg.MaxWinRMSOAPResponseBytes+1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		_, _ = w.Write([]byte(oversizedBody))
	}))
	defer server.Close()

	_, err := collectorspkg.WinRMSOAPRequest(context.Background(), server.Client(), server.URL, "Administrator", "password", "<Envelope/>")
	if err == nil {
		t.Fatal("expected oversized SOAP response error")
	}
	if !strings.Contains(err.Error(), "response exceeded") {
		t.Fatalf("expected response size limit error, got: %v", err)
	}
}
