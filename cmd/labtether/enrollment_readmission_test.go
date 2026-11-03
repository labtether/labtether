package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/enrollment"
	"github.com/labtether/labtether/internal/persistence"
)

// A removed device used to be refused for good with "agent identity was
// decommissioned", which no user saw. Owner decision 2026-11-03: a new
// single-use token re-admits it, and every refusal says what to do in words
// the agent shows.
func TestEnrollAfterRemovalNeedsANewSingleUseTokenAndSaysSo(t *testing.T) {
	disableTailscaleResolutionForTest(t)
	sut := newTestAPIServer(t)
	enroll := func(rawToken string) *httptest.ResponseRecorder {
		payload, err := json.Marshal(enrollment.EnrollRequest{EnrollmentToken: rawToken, Hostname: "node-b", Platform: "linux"})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(payload))
		req.Host = "localhost:8080"
		req.RemoteAddr = "127.0.0.1:12345"
		rec := httptest.NewRecorder()
		sut.handleEnroll(rec, req)
		return rec
	}
	token := func(maxUses int) string {
		raw, _ := mustCreateEnrollmentTokenWithRequest(t, sut, enrollment.CreateTokenRequest{Label: "t", TTLHours: 1, MaxUses: maxUses, Scope: enrollment.TokenScopeUnplaced})
		return raw
	}
	expect := func(rec *httptest.ResponseRecorder, code int, words string) {
		t.Helper()
		if rec.Code != code || !strings.Contains(rec.Body.String(), words) {
			t.Fatalf("got %d %s, want %d with %q", rec.Code, rec.Body.String(), code, words)
		}
	}
	expect(enroll(token(1)), http.StatusOK, "agent_token")
	time.Sleep(2 * time.Millisecond)
	transactions := sut.enrollmentStore.(persistence.AgentEnrollmentTransactionStore)
	if err := transactions.DecommissionAgentAsset(context.Background(), "node-b"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	expect(enroll(token(5)), http.StatusConflict, "This device was removed from LabTether. Only a new single-use enrollment token made after the removal can add it back.")
	fresh := token(1)
	expect(enroll(fresh), http.StatusOK, "agent_token")
	expect(enroll(fresh), http.StatusUnauthorized, "This enrollment token was already used. Make a new one in Devices, Add device.")
	expect(enroll("lt_enr_not-a-real-token"), http.StatusUnauthorized, "LabTether does not know this enrollment token.")
}
