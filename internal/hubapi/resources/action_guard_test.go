package resources

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
)

func TestAssetMutationHandlersHonorMaintenanceBlockActions(t *testing.T) {
	evaluations := 0
	deps := &Deps{
		EvaluateAssetGuardrails: func(assetID string, at time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
			evaluations++
			if assetID != "asset-1" || at.IsZero() {
				t.Fatalf("unexpected guard evaluation asset=%q at=%s", assetID, at)
			}
			return groupfeatures.GroupMaintenanceGuardrails{
				GroupID:      "group-1",
				BlockActions: true,
				ActiveWindows: []groupmaintenance.MaintenanceWindow{{
					ID:      "window-1",
					GroupID: "group-1",
				}},
			}, nil
		},
	}

	tests := []struct {
		name   string
		method string
		path   string
		run    func(http.ResponseWriter, *http.Request)
	}{
		{name: "wake on LAN", method: http.MethodPost, path: "/assets/asset-1/wake", run: func(w http.ResponseWriter, r *http.Request) { deps.HandleWakeOnLAN(w, r, "asset-1") }},
		{name: "service action", method: http.MethodPost, path: "/services/asset-1/restart", run: func(w http.ResponseWriter, r *http.Request) { deps.handleServiceAction(w, r, "asset-1", "restart") }},
		{name: "process kill", method: http.MethodPost, path: "/processes/asset-1/kill", run: func(w http.ResponseWriter, r *http.Request) { deps.handleProcessKill(w, r, "asset-1") }},
		{name: "network apply", method: http.MethodPost, path: "/network/asset-1/apply", run: func(w http.ResponseWriter, r *http.Request) { deps.handleNetworkAction(w, r, "asset-1", "apply") }},
		{name: "package install", method: http.MethodPost, path: "/packages/asset-1/install", run: func(w http.ResponseWriter, r *http.Request) { deps.handlePackageAction(w, r, "asset-1", "install") }},
		{name: "file upload", method: http.MethodPost, path: "/files/asset-1/upload", run: func(w http.ResponseWriter, r *http.Request) { deps.HandleFileUpload(w, r, "asset-1") }},
		{name: "file mkdir", method: http.MethodPost, path: "/files/asset-1/mkdir", run: func(w http.ResponseWriter, r *http.Request) { deps.HandleFileMkdir(w, r, "asset-1") }},
		{name: "file delete", method: http.MethodDelete, path: "/files/asset-1/delete", run: func(w http.ResponseWriter, r *http.Request) { deps.HandleFileDelete(w, r, "asset-1") }},
		{name: "file rename", method: http.MethodPost, path: "/files/asset-1/rename", run: func(w http.ResponseWriter, r *http.Request) { deps.HandleFileRename(w, r, "asset-1") }},
		{name: "file copy", method: http.MethodPost, path: "/files/asset-1/copy", run: func(w http.ResponseWriter, r *http.Request) { deps.HandleFileCopy(w, r, "asset-1") }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, nil)
			test.run(recorder, request)
			if recorder.Code != http.StatusLocked {
				t.Fatalf("status = %d, want 423: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
	if evaluations != len(tests) {
		t.Fatalf("guard evaluations = %d, want %d", evaluations, len(tests))
	}
}

func TestPackageMutationHonorsMaintenanceBlockUpdates(t *testing.T) {
	deps := &Deps{
		EvaluateAssetGuardrails: func(assetID string, at time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
			return groupfeatures.GroupMaintenanceGuardrails{
				GroupID:      "group-updates",
				BlockUpdates: true,
				ActiveWindows: []groupmaintenance.MaintenanceWindow{{
					ID:           "window-updates",
					GroupID:      "group-updates",
					BlockUpdates: true,
				}},
			}, nil
		},
	}
	for _, action := range []string{"install", "remove", "upgrade"} {
		t.Run(action, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/packages/asset-1/"+action, nil)
			deps.handlePackageAction(recorder, request, "asset-1", action)
			if recorder.Code != http.StatusLocked {
				t.Fatalf("status = %d, want 423: %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
