package maintenanceguard

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
)

func TestEnforceAssetActionAllowsWithoutEvaluatorOrBlock(t *testing.T) {
	for _, evaluate := range []EvaluateAssetFunc{
		nil,
		func(string, time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
			return groupfeatures.GroupMaintenanceGuardrails{GroupID: "group-1"}, nil
		},
	} {
		recorder := httptest.NewRecorder()
		if !EnforceAssetAction(recorder, "asset-1", evaluate) {
			t.Fatal("expected action to be allowed")
		}
		if recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
			t.Fatalf("unexpected response while allowing action: %d %q", recorder.Code, recorder.Body.String())
		}
	}
}

func TestEnforceAssetActionFailsClosedOnEvaluationError(t *testing.T) {
	recorder := httptest.NewRecorder()
	allowed := EnforceAssetAction(recorder, "asset-1", func(string, time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
		return groupfeatures.GroupMaintenanceGuardrails{}, errors.New("store unavailable")
	})
	if allowed {
		t.Fatal("expected evaluation failure to deny action")
	}
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
}

func TestEnforceAssetActionReturnsCanonicalLockedResponse(t *testing.T) {
	recorder := httptest.NewRecorder()
	allowed := EnforceAssetAction(recorder, "asset-1", func(assetID string, at time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
		if assetID != "asset-1" || at.IsZero() {
			t.Fatalf("unexpected evaluation input asset=%q at=%s", assetID, at)
		}
		return groupfeatures.GroupMaintenanceGuardrails{
			GroupID:      "group-1",
			BlockActions: true,
			ActiveWindows: []groupmaintenance.MaintenanceWindow{{
				ID:      "window-1",
				GroupID: "group-1",
			}},
		}, nil
	})
	if allowed {
		t.Fatal("expected maintenance window to deny action")
	}
	if recorder.Code != http.StatusLocked {
		t.Fatalf("status = %d, want 423", recorder.Code)
	}
	body := recorder.Body.String()
	for _, expected := range []string{`"error":"maintenance_blocked"`, `"group_id":"group-1"`, `"id":"window-1"`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("response %q missing %q", body, expected)
		}
	}
}

func TestEnforceAssetUpdateHonorsDedicatedUpdateGuard(t *testing.T) {
	recorder := httptest.NewRecorder()
	allowed := EnforceAssetUpdate(recorder, "asset-1", func(string, time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
		return groupfeatures.GroupMaintenanceGuardrails{
			GroupID:      "group-updates",
			BlockUpdates: true,
			ActiveWindows: []groupmaintenance.MaintenanceWindow{{
				ID:           "window-updates",
				GroupID:      "group-updates",
				BlockUpdates: true,
			}},
		}, nil
	})
	if allowed {
		t.Fatal("expected block_updates to deny update")
	}
	if recorder.Code != http.StatusLocked {
		t.Fatalf("status = %d, want 423", recorder.Code)
	}
	body := recorder.Body.String()
	for _, expected := range []string{`"error":"maintenance_blocked"`, `"group_id":"group-updates"`, `updates are blocked`} {
		if !strings.Contains(body, expected) {
			t.Fatalf("response %q missing %q", body, expected)
		}
	}
}

func TestEnforceAssetActionDoesNotTreatBlockUpdatesAsBlockActions(t *testing.T) {
	recorder := httptest.NewRecorder()
	allowed := EnforceAssetAction(recorder, "asset-1", func(string, time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
		return groupfeatures.GroupMaintenanceGuardrails{BlockUpdates: true}, nil
	})
	if !allowed {
		t.Fatal("ordinary action should not be denied solely by block_updates")
	}
}
