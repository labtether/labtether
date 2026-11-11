package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/labtether/labtether/internal/hubcollector"
)

func setUpTrueNASForReadTests(t *testing.T, handler func(method string, params []any) (any, *trueNASRPCError)) *apiServer {
	t.Helper()
	server := newTrueNASRPCServer(t, handler)
	t.Cleanup(server.Close)
	sut := newTestAPIServer(t)
	createTrueNASCredentialProfile(t, sut, "cred-truenas-1", "api-key-1", server.URL)
	configureTrueNASCollectors(t, sut, hubcollector.Collector{
		ID: "collector-truenas-1", AssetID: "truenas-cluster-1", CollectorType: hubcollector.CollectorTypeTrueNAS, Enabled: true,
		Config: map[string]any{"base_url": server.URL, "credential_id": "cred-truenas-1", "skip_verify": true},
	})
	seedTrueNASAsset(t, sut, "truenas-host-nas01", "collector-truenas-1")
	return sut
}

// TrueNAS refuses a call for a login without the role with "Not authorized"
// (EACCES) in the error's data. The Hub read it as an argument-shape mismatch,
// retried it 15 times with fresh logins, then answered "An internal error
// occurred.".
func TestTrueNASDiskHealthRefusedForTheLoginSaysSo(t *testing.T) {
	var diskQueries atomic.Int32
	sut := setUpTrueNASForReadTests(t, func(method string, _ []any) (any, *trueNASRPCError) {
		if method == "disk.query" {
			diskQueries.Add(1)
			return nil, &trueNASRPCError{Code: -32001, Message: "Method call error",
				Data: map[string]any{"error": 13, "errname": "EACCES", "reason": "Not authorized"}}
		}
		return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
	})

	rec := httptest.NewRecorder()
	sut.handleTrueNASAssets(rec, httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-nas01/smart", nil))
	var body struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	want := "TrueNAS refused this for the saved login. Give that login access to it on TrueNAS."
	if rec.Code != http.StatusConflict || body.Error != want {
		t.Fatalf("got %d %s, want 409 %q", rec.Code, rec.Body.String(), want)
	}
	if n := diskQueries.Load(); n != 1 {
		t.Fatalf("disk.query was called %d times, want 1 (a refusal is not retried)", n)
	}
}

// The Disks overview treats a TrueNAS without smart.test.results as having no
// test history; the per-disk SMART details answered 502 instead.
func TestTrueNASSMARTDetailsWithoutTestHistory(t *testing.T) {
	sut := setUpTrueNASForReadTests(t, func(string, []any) (any, *trueNASRPCError) {
		return nil, &trueNASRPCError{Code: -32601, Message: "Method not found"}
	})

	rec := httptest.NewRecorder()
	sut.handleTrueNASAssets(rec, httptest.NewRequest(http.MethodGet, "/truenas/assets/truenas-host-nas01/disks/sda/smart-details", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "This TrueNAS keeps no SMART test history.") {
		t.Fatalf("got %d %s, want 200 with no history in words", rec.Code, rec.Body.String())
	}
}
