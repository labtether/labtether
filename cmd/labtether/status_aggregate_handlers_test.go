package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/runtimesettings"
	"github.com/labtether/labtether/internal/terminal"
)

func TestStatusAggregateSummaryUsesWebServices(t *testing.T) {
	sut := newTestAPIServer(t)

	_, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "host-1",
		Type:    "node",
		Name:    "Host 1",
		Source:  "agent",
		Status:  "online",
	})
	if err != nil {
		t.Fatalf("failed to seed asset: %v", err)
	}

	report := agentmgr.WebServiceReportData{
		HostAssetID: "host-1",
		Services: []agentmgr.DiscoveredWebService{
			{
				ID:          "svc-up",
				Name:        "Svc Up",
				Category:    "Other",
				URL:         "http://host-1:8080",
				Source:      "docker",
				Status:      "up",
				HostAssetID: "host-1",
			},
			{
				ID:          "svc-down",
				Name:        "Svc Down",
				Category:    "Other",
				URL:         "http://host-1:8081",
				Source:      "docker",
				Status:      "down",
				HostAssetID: "host-1",
			},
		},
	}
	raw, _ := json.Marshal(report)
	sut.webServiceCoordinator.HandleReport("host-1", agentmgr.Message{
		Type: agentmgr.MsgWebServiceReport,
		Data: raw,
	})

	_, err = sut.webServiceCoordinator.SaveOverride(persistence.WebServiceOverride{
		HostAssetID: "host-1",
		ServiceID:   "svc-down",
		Hidden:      true,
	})
	if err != nil {
		t.Fatalf("failed to hide service via override: %v", err)
	}

	live := sut.buildStatusAggregateLiveResponse(context.Background(), "")
	if live.Summary.ServicesTotal != 1 {
		t.Fatalf("services total = %d, want 1 visible service", live.Summary.ServicesTotal)
	}
	if live.Summary.ServicesUp != 1 {
		t.Fatalf("services up = %d, want 1", live.Summary.ServicesUp)
	}
}

func TestStatusEndpointTargetsOmitsNodeAgentForDefaultSource(t *testing.T) {
	targets := statusEndpointTargets(
		statusResolvedRoutingURL{
			URL:    "http://localhost:8080",
			Source: runtimesettings.SourceDefault,
		},
		statusResolvedRoutingURL{
			URL:    "http://localhost:8090",
			Source: runtimesettings.SourceDefault,
		},
		true,
	)

	if len(targets) != 1 {
		t.Fatalf("expected only LabTether endpoint, got %d targets", len(targets))
	}
	if targets[0].Name != "LabTether" {
		t.Fatalf("expected LabTether endpoint, got %q", targets[0].Name)
	}
	if targets[0].URL != "http://localhost:8080/healthz" {
		t.Fatalf("expected LabTether health URL, got %q", targets[0].URL)
	}
}

func TestStatusEndpointTargetsOmitsNodeAgentWhenHubNotDockerHosted(t *testing.T) {
	targets := statusEndpointTargets(
		statusResolvedRoutingURL{
			URL:    "https://localhost:8443",
			Source: runtimesettings.SourceDefault,
		},
		statusResolvedRoutingURL{
			URL:    "https://labtether-agent:8090",
			Source: runtimesettings.SourceUI,
		},
		false,
	)

	if len(targets) != 1 {
		t.Fatalf("expected only LabTether endpoint when hub is not docker-hosted, got %d targets", len(targets))
	}
	if targets[0].Name != "LabTether" {
		t.Fatalf("expected LabTether endpoint, got %q", targets[0].Name)
	}
}

func TestStatusEndpointTargetsIncludesNodeAgentForConfiguredSourceWhenHubDockerHosted(t *testing.T) {
	testCases := []struct {
		name   string
		source runtimesettings.Source
	}{
		{name: "docker source", source: runtimesettings.SourceDocker},
		{name: "ui override source", source: runtimesettings.SourceUI},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			targets := statusEndpointTargets(
				statusResolvedRoutingURL{
					URL:    "https://labtether:8443",
					Source: runtimesettings.SourceDocker,
				},
				statusResolvedRoutingURL{
					URL:    "https://labtether-agent:8090",
					Source: tc.source,
				},
				true,
			)

			if len(targets) != 2 {
				t.Fatalf("expected LabTether + Node Agent endpoints, got %d targets", len(targets))
			}
			if targets[0].Name != "LabTether" || targets[0].URL != "https://labtether:8443/healthz" {
				t.Fatalf("unexpected LabTether target: %+v", targets[0])
			}
			if targets[1].Name != "Node Agent" || targets[1].URL != "https://labtether-agent:8090/healthz" {
				t.Fatalf("unexpected Node Agent target: %+v", targets[1])
			}
		})
	}
}

func TestHandleStatusAggregateScopesTerminalDataAndETagsByActor(t *testing.T) {
	sut := newTestAPIServer(t)

	sessionA, err := sut.terminalStore.CreateSession(terminal.CreateSessionRequest{
		ActorID: "actor-a",
		Target:  "host-a",
		Mode:    "interactive",
	})
	if err != nil {
		t.Fatalf("create actor-a session: %v", err)
	}
	sessionB, err := sut.terminalStore.CreateSession(terminal.CreateSessionRequest{
		ActorID: "actor-b",
		Target:  "host-b",
		Mode:    "interactive",
	})
	if err != nil {
		t.Fatalf("create actor-b session: %v", err)
	}
	if _, err := sut.terminalStore.AddCommand(sessionA.ID, terminal.CreateCommandRequest{
		ActorID: "actor-a",
		Command: "uptime",
	}, sessionA.Target, sessionA.Mode); err != nil {
		t.Fatalf("add actor-a command: %v", err)
	}
	if _, err := sut.terminalStore.AddCommand(sessionB.ID, terminal.CreateCommandRequest{
		ActorID: "actor-b",
		Command: "hostname",
	}, sessionB.Target, sessionB.Mode); err != nil {
		t.Fatalf("add actor-b command: %v", err)
	}
	if err := sut.auditStore.Append(audit.Event{ID: "audit-a", ActorID: "actor-a", Type: "terminal.command.created"}); err != nil {
		t.Fatalf("append actor-a audit: %v", err)
	}
	if err := sut.auditStore.Append(audit.Event{ID: "audit-b", ActorID: "actor-b", Type: "terminal.command.created"}); err != nil {
		t.Fatalf("append actor-b audit: %v", err)
	}

	handler := sut.handleStatusAggregate(nil, nil)

	ownerReq := httptest.NewRequest(http.MethodGet, "/status/aggregate", nil)
	ownerRec := httptest.NewRecorder()
	handler(ownerRec, ownerReq)
	if ownerRec.Code != http.StatusOK {
		t.Fatalf("expected owner aggregate 200, got %d: %s", ownerRec.Code, ownerRec.Body.String())
	}
	ownerETag := ownerRec.Header().Get("ETag")
	if ownerETag == "" {
		t.Fatal("expected owner aggregate ETag")
	}

	actorReq := httptest.NewRequest(http.MethodGet, "/status/aggregate", nil)
	actorReq.Header.Set("If-None-Match", ownerETag)
	actorReq = actorReq.WithContext(contextWithUserID(actorReq.Context(), "actor-a"))
	actorRec := httptest.NewRecorder()
	handler(actorRec, actorReq)
	if actorRec.Code != http.StatusOK {
		t.Fatalf("expected actor aggregate 200 with owner ETag, got %d", actorRec.Code)
	}

	var actorResponse struct {
		Sessions       []terminal.Session `json:"sessions"`
		RecentCommands []terminal.Command `json:"recentCommands"`
		RecentAudit    []audit.Event      `json:"recentAudit"`
	}
	if err := json.Unmarshal(actorRec.Body.Bytes(), &actorResponse); err != nil {
		t.Fatalf("decode actor aggregate: %v", err)
	}
	if len(actorResponse.Sessions) != 1 || actorResponse.Sessions[0].ActorID != "actor-a" {
		t.Fatalf("expected only actor-a session, got %+v", actorResponse.Sessions)
	}
	if len(actorResponse.RecentCommands) != 1 || actorResponse.RecentCommands[0].ActorID != "actor-a" {
		t.Fatalf("expected only actor-a recent command, got %+v", actorResponse.RecentCommands)
	}
	if len(actorResponse.RecentAudit) != 1 || actorResponse.RecentAudit[0].ActorID != "actor-a" {
		t.Fatalf("expected only actor-a recent audit, got %+v", actorResponse.RecentAudit)
	}

	actorETag := actorRec.Header().Get("ETag")
	if actorETag == "" {
		t.Fatal("expected actor aggregate ETag")
	}
	if actorETag == ownerETag {
		t.Fatal("expected actor-scoped aggregate ETag to differ from owner ETag")
	}

	actorCachedReq := httptest.NewRequest(http.MethodGet, "/status/aggregate", nil)
	actorCachedReq.Header.Set("If-None-Match", actorETag)
	actorCachedReq = actorCachedReq.WithContext(contextWithUserID(actorCachedReq.Context(), "actor-a"))
	actorCachedRec := httptest.NewRecorder()
	handler(actorCachedRec, actorCachedReq)
	if actorCachedRec.Code != http.StatusNotModified {
		t.Fatalf("expected actor-scoped ETag cache to return 304, got %d", actorCachedRec.Code)
	}
}

func TestHandleStatusAggregateEnforcesScopesAndAssetAllowlist(t *testing.T) {
	sut := newTestAPIServer(t)
	for _, assetID := range []string{"asset-a", "asset-b"} {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: assetID,
			Name:    assetID,
			Type:    "node",
			Source:  "agent",
			Status:  "online",
		}); err != nil {
			t.Fatalf("seed %s: %v", assetID, err)
		}
	}

	const actorID = "apikey:key-status"
	for _, assetID := range []string{"asset-a", "asset-b"} {
		session, err := sut.terminalStore.CreateSession(terminal.CreateSessionRequest{
			ActorID: actorID,
			Target:  assetID,
			Mode:    "interactive",
		})
		if err != nil {
			t.Fatalf("create %s session: %v", assetID, err)
		}
		if _, err := sut.terminalStore.AddCommand(session.ID, terminal.CreateCommandRequest{
			ActorID: actorID,
			Command: "uptime",
		}, assetID, session.Mode); err != nil {
			t.Fatalf("create %s command: %v", assetID, err)
		}
	}

	fullCtx := contextWithPrincipal(context.Background(), actorID, "operator")
	fullCtx = contextWithScopes(fullCtx, []string{"hub:read", "assets:read", "logs:read", "terminal:read"})
	fullCtx = contextWithAllowedAssets(fullCtx, []string{"asset-a"})

	handler := sut.handleStatusAggregate(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/status/aggregate", nil).WithContext(fullCtx)
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected aggregate 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var filtered struct {
		Assets           []assets.Asset     `json:"assets"`
		Sessions         []terminal.Session `json:"sessions"`
		RecentCommands   []terminal.Command `json:"recentCommands"`
		RecentLogs       []json.RawMessage  `json:"recentLogs"`
		Telemetry        []json.RawMessage  `json:"telemetryOverview"`
		RecentAudit      []json.RawMessage  `json:"recentAudit"`
		DeadLetters      []json.RawMessage  `json:"deadLetters"`
		UpdatePlans      []json.RawMessage  `json:"updatePlans"`
		GroupReliability []json.RawMessage  `json:"groupReliability"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &filtered); err != nil {
		t.Fatalf("decode filtered aggregate: %v", err)
	}
	if len(filtered.Assets) != 1 || filtered.Assets[0].ID != "asset-a" {
		t.Fatalf("expected only asset-a, got %+v", filtered.Assets)
	}
	if len(filtered.Sessions) != 1 || filtered.Sessions[0].Target != "asset-a" {
		t.Fatalf("expected only asset-a session, got %+v", filtered.Sessions)
	}
	if len(filtered.RecentCommands) != 1 || filtered.RecentCommands[0].Target != "asset-a" {
		t.Fatalf("expected only asset-a command, got %+v", filtered.RecentCommands)
	}
	if len(filtered.Telemetry) != 0 || len(filtered.RecentAudit) != 0 || len(filtered.DeadLetters) != 0 || len(filtered.UpdatePlans) != 0 || len(filtered.GroupReliability) != 0 {
		t.Fatalf("aggregate exposed data without its underlying scope: %+v", filtered)
	}

	hubOnlyCtx := contextWithPrincipal(context.Background(), actorID, "operator")
	hubOnlyCtx = contextWithScopes(hubOnlyCtx, []string{"hub:read"})
	hubOnlyCtx = contextWithAllowedAssets(hubOnlyCtx, []string{"asset-a"})
	hubOnlyReq := httptest.NewRequest(http.MethodGet, "/status/aggregate", nil).WithContext(hubOnlyCtx)
	hubOnlyRec := httptest.NewRecorder()
	handler(hubOnlyRec, hubOnlyReq)
	if hubOnlyRec.Code != http.StatusOK {
		t.Fatalf("expected hub-only aggregate 200, got %d: %s", hubOnlyRec.Code, hubOnlyRec.Body.String())
	}
	var hubOnly struct {
		Assets         []assets.Asset     `json:"assets"`
		Sessions       []terminal.Session `json:"sessions"`
		RecentCommands []terminal.Command `json:"recentCommands"`
	}
	if err := json.Unmarshal(hubOnlyRec.Body.Bytes(), &hubOnly); err != nil {
		t.Fatalf("decode hub-only aggregate: %v", err)
	}
	if len(hubOnly.Assets) != 0 || len(hubOnly.Sessions) != 0 || len(hubOnly.RecentCommands) != 0 {
		t.Fatalf("hub:read implicitly exposed underlying resources: %+v", hubOnly)
	}
}
