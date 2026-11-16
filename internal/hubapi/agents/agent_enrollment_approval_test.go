package agents_test

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/labtether/labtether/internal/agentidentity"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	agentspkg "github.com/labtether/labtether/internal/hubapi/agents"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	"github.com/labtether/labtether/internal/persistence"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleApproveAgentRequiresIdentityVerified(t *testing.T) {
	sut := hubdeps.New(t)
	sut.PendingAgents = agentspkg.NewPendingAgents()
	sut.PendingAgents.Add(&agentspkg.PendingAgent{
		AssetID:          "pending-charlie-1",
		Hostname:         "charlie",
		Platform:         "linux",
		IdentityVerified: false,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/approve", bytes.NewReader([]byte(`{"asset_id":"pending-charlie-1"}`)))
	rec := httptest.NewRecorder()
	sut.AgentsDeps().HandleApproveAgent(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 when identity is unverified, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleApproveAgentRejectsRetiredIdentity(t *testing.T) {
	sut := hubdeps.New(t)
	transactions, ok := sut.EnrollmentStore.(persistence.AgentEnrollmentTransactionStore)
	if !ok {
		t.Fatal("test enrollment store lacks transaction interface")
	}
	if _, err := sut.AssetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "retired-approval", Name: "retired-approval", Type: "node", Source: "agent", Status: "online",
	}); err != nil {
		t.Fatal(err)
	}
	if err := transactions.DecommissionAgentAsset(context.Background(), "retired-approval"); err != nil {
		t.Fatal(err)
	}

	sut.PendingAgents = agentspkg.NewPendingAgents()
	verifiedAt := time.Now().UTC()
	sut.PendingAgents.Add(&agentspkg.PendingAgent{
		AssetID:            "pending-retired-approval",
		Hostname:           "retired-approval",
		Platform:           "linux",
		DeviceFingerprint:  "LT-RETIRED-APPROVAL",
		DeviceKeyAlg:       agentidentity.KeyAlgorithmEd25519,
		IdentityVerified:   true,
		IdentityVerifiedAt: &verifiedAt,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/approve", bytes.NewReader([]byte(`{"asset_id":"pending-retired-approval"}`)))
	rec := httptest.NewRecorder()
	sut.AgentsDeps().HandleApproveAgent(rec, req)

	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "This device was removed from LabTether") {
		t.Fatalf("retired approval status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, exists := sut.PendingAgents.Get("pending-retired-approval"); !exists {
		t.Fatal("retired approval removed the pending agent instead of releasing the decision")
	}
}

func TestDecodePendingEnrollmentAssetID(t *testing.T) {
	t.Run("rejects malformed body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/approve", bytes.NewReader([]byte(`{`)))
		rec := httptest.NewRecorder()

		if _, ok := agentspkg.DecodePendingEnrollmentAssetID(rec, req); ok {
			t.Fatalf("expected malformed body to be rejected")
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("rejects missing asset_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/approve", bytes.NewReader([]byte(`{"asset_id":"  "}`)))
		rec := httptest.NewRecorder()

		if _, ok := agentspkg.DecodePendingEnrollmentAssetID(rec, req); ok {
			t.Fatalf("expected empty asset_id to be rejected")
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestResolveApprovedAssetID(t *testing.T) {
	if got := agentspkg.ResolveApprovedAssetID(agentspkg.PendingAgentInfo{Hostname: "lab-host-01"}, "pending-lab-host-01"); got != "lab-host-01" {
		t.Fatalf("expected hostname to become stable asset id, got %q", got)
	}
	if got := agentspkg.ResolveApprovedAssetID(agentspkg.PendingAgentInfo{Hostname: "unknown"}, "pending-unknown-1"); got != "pending-unknown-1" {
		t.Fatalf("expected unknown hostname to fall back to pending asset id, got %q", got)
	}
	if got := agentspkg.ResolveApprovedAssetID(agentspkg.PendingAgentInfo{}, "pending-empty-1"); got != "pending-empty-1" {
		t.Fatalf("expected empty hostname to fall back to pending asset id, got %q", got)
	}
}

func TestHandleApproveAgentErrorPaths(t *testing.T) {
	t.Run("rejects non-POST method", func(t *testing.T) {
		sut := hubdeps.New(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/approve", nil)
		rec := httptest.NewRecorder()
		sut.AgentsDeps().HandleApproveAgent(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})

	t.Run("returns not found for missing pending agent", func(t *testing.T) {
		sut := hubdeps.New(t)
		sut.PendingAgents = agentspkg.NewPendingAgents()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/approve", bytes.NewReader([]byte(`{"asset_id":"pending-missing-1"}`)))
		rec := httptest.NewRecorder()
		sut.AgentsDeps().HandleApproveAgent(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})

	t.Run("returns service unavailable without enrollment store", func(t *testing.T) {
		sut := hubdeps.New(t)
		sut.PendingAgents = agentspkg.NewPendingAgents()
		sut.EnrollmentStore = nil
		sut.PendingAgents.Add(&agentspkg.PendingAgent{
			AssetID:          "pending-foxtrot-1",
			Hostname:         "foxtrot",
			IdentityVerified: true,
		})

		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/approve", bytes.NewReader([]byte(`{"asset_id":"pending-foxtrot-1"}`)))
		rec := httptest.NewRecorder()
		sut.AgentsDeps().HandleApproveAgent(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503, got %d", rec.Code)
		}
	})
}

func TestHandleApproveAgentSuccess(t *testing.T) {
	sut := hubdeps.New(t)
	sut.PendingAgents = agentspkg.NewPendingAgents()

	serverConn, clientConn, cleanup := testutil.CreateWSPairForNetworkTest(t)
	defer cleanup()

	verifiedAt := time.Now().UTC()
	sut.PendingAgents.Add(&agentspkg.PendingAgent{
		AssetID:            "pending-golf-1",
		Hostname:           "golf",
		Platform:           "linux",
		ConnectedAt:        verifiedAt,
		DeviceFingerprint:  "sha256:test",
		DeviceKeyAlg:       agentidentity.KeyAlgorithmEd25519,
		IdentityVerified:   true,
		IdentityVerifiedAt: &verifiedAt,
		Conn:               serverConn,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/approve", bytes.NewReader([]byte(`{"asset_id":"pending-golf-1"}`)))
	rec := httptest.NewRecorder()
	sut.AgentsDeps().HandleApproveAgent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "approved" {
		t.Fatalf("expected approved status, got %v", resp["status"])
	}
	if resp["asset_id"] != "golf" {
		t.Fatalf("expected stable asset_id golf, got %v", resp["asset_id"])
	}

	if _, ok := sut.PendingAgents.Get("pending-golf-1"); ok {
		t.Fatalf("expected pending agent to be removed after approval")
	}

	tokens, err := sut.EnrollmentStore.ListAgentTokens(10)
	if err != nil {
		t.Fatalf("list agent tokens: %v", err)
	}
	if len(tokens) != 1 || tokens[0].AssetID != "golf" {
		t.Fatalf("expected one issued token for golf, got %+v", tokens)
	}

	if _, ok, err := sut.AssetStore.GetAsset("golf"); err != nil {
		t.Fatalf("get asset: %v", err)
	} else if !ok {
		t.Fatalf("expected approved asset to be upserted")
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg agentmgr.Message
	if err := clientConn.ReadJSON(&msg); err != nil {
		t.Fatalf("read approval message: %v", err)
	}
	if msg.Type != agentmgr.MsgEnrollmentApproved {
		t.Fatalf("expected enrollment.approved message, got %q", msg.Type)
	}
	var approved agentmgr.EnrollmentApprovedData
	if err := json.Unmarshal(msg.Data, &approved); err != nil {
		t.Fatalf("decode approval payload: %v", err)
	}
	if approved.AssetID != "golf" {
		t.Fatalf("expected approved asset id golf, got %q", approved.AssetID)
	}
	if strings.TrimSpace(approved.Token) == "" {
		t.Fatalf("expected issued token in approval payload")
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := clientConn.ReadMessage(); err == nil {
		t.Fatal("approved pending socket remained open after finalization")
	}
}

func TestHandleApproveAgentFinalizesWithIndependentContext(t *testing.T) {
	sut := hubdeps.New(t)
	sut.PendingAgents = agentspkg.NewPendingAgents()
	underlyingEnrollment := sut.EnrollmentStore
	transactions, ok := underlyingEnrollment.(persistence.AgentEnrollmentTransactionStore)
	if !ok {
		t.Fatal("test enrollment store lacks transaction interface")
	}
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	wrapped := &cancelOnFinalizeEnrollmentStore{
		EnrollmentStore:                 underlyingEnrollment,
		AgentEnrollmentTransactionStore: transactions,
		cancelRequest:                   cancelRequest,
	}
	sut.EnrollmentStore = wrapped

	serverConn, clientConn, cleanup := testutil.CreateWSPairForNetworkTest(t)
	defer cleanup()
	verifiedAt := time.Now().UTC()
	sut.PendingAgents.Add(&agentspkg.PendingAgent{
		AssetID:            "pending-cancel-1",
		Hostname:           "cancel-node",
		Platform:           "linux",
		DeviceFingerprint:  "LT-CANCEL-CONTEXT",
		DeviceKeyAlg:       agentidentity.KeyAlgorithmEd25519,
		IdentityVerified:   true,
		IdentityVerifiedAt: &verifiedAt,
		Conn:               serverConn,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/approve", bytes.NewReader([]byte(`{"asset_id":"pending-cancel-1"}`))).WithContext(requestCtx)
	rec := httptest.NewRecorder()
	sut.AgentsDeps().HandleApproveAgent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("request cancellation prevented finalization: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !wrapped.finalizeContextWasLive {
		t.Fatal("finalization reused the canceled operator request context")
	}
	tokens, err := underlyingEnrollment.ListAgentTokens(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens) != 1 || tokens[0].AssetID != "cancel-node" || tokens[0].Status != "active" {
		t.Fatalf("independent finalization token state=%+v", tokens)
	}
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var approval agentmgr.Message
	if err := clientConn.ReadJSON(&approval); err != nil || approval.Type != agentmgr.MsgEnrollmentApproved {
		t.Fatalf("approval message=%+v err=%v", approval, err)
	}
}

func TestHandleRejectAgentSuccess(t *testing.T) {
	sut := hubdeps.New(t)
	sut.PendingAgents = agentspkg.NewPendingAgents()

	serverConn, clientConn, cleanup := testutil.CreateWSPairForNetworkTest(t)
	defer cleanup()

	sut.PendingAgents.Add(&agentspkg.PendingAgent{
		AssetID:     "pending-hotel-1",
		Hostname:    "hotel",
		Platform:    "linux",
		ConnectedAt: time.Now().UTC(),
		Conn:        serverConn,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/reject", bytes.NewReader([]byte(`{"asset_id":"pending-hotel-1"}`)))
	rec := httptest.NewRecorder()
	sut.AgentsDeps().HandleRejectAgent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, ok := sut.PendingAgents.Get("pending-hotel-1"); ok {
		t.Fatalf("expected pending agent to be removed after rejection")
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg agentmgr.Message
	if err := clientConn.ReadJSON(&msg); err != nil {
		t.Fatalf("read rejection message: %v", err)
	}
	if msg.Type != agentmgr.MsgEnrollmentRejected {
		t.Fatalf("expected enrollment.rejected message, got %q", msg.Type)
	}
	var rejected agentmgr.EnrollmentRejectedData
	if err := json.Unmarshal(msg.Data, &rejected); err != nil {
		t.Fatalf("decode rejection payload: %v", err)
	}
	if !strings.Contains(rejected.Reason, "rejected") {
		t.Fatalf("expected rejection reason, got %q", rejected.Reason)
	}

	if _, _, err := clientConn.ReadMessage(); err == nil {
		t.Fatalf("expected close frame after rejection")
	}

	// Reject sticks: the same device asking again is refused, not listed in
	// Pending again (it was back within seconds).
	again := httptest.NewRequest(http.MethodGet, "/ws/agent", nil)
	again.Header.Set("X-Request-Enrollment", "true")
	again.Header.Set("X-Hostname", "hotel")
	againRec := httptest.NewRecorder()
	sut.AgentsDeps().HandlePendingEnrollment(againRec, again)
	if againRec.Code != http.StatusForbidden || !strings.Contains(againRec.Body.String(), "An admin rejected this device") {
		t.Fatalf("rejected device asked again: %d %s", againRec.Code, againRec.Body.String())
	}
	if sut.PendingAgents.Count() != 0 {
		t.Fatalf("rejected device is back in Pending")
	}
}
