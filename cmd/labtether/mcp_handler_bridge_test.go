package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/assets"
	dockerconnector "github.com/labtether/labtether/internal/connectors/docker"
	"github.com/labtether/labtether/internal/edges"
	"github.com/labtether/labtether/internal/groups"
	respkg "github.com/labtether/labtether/internal/hubapi/resources"
	"github.com/labtether/labtether/internal/savedactions"
	"github.com/labtether/labtether/internal/schedules"
	"github.com/labtether/labtether/internal/updates"
)

func TestMCPListServicesUsesCorrelatedTypedProtocol(t *testing.T) {
	sut := newTestAPIServer(t)
	sut.agentMgr = agentmgr.NewManager()
	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()
	sut.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-svc", "linux"))
	defer sut.agentMgr.Unregister("node-svc")

	done := make(chan struct{})
	go func() {
		defer close(done)
		var outbound agentmgr.Message
		if err := clientConn.ReadJSON(&outbound); err != nil {
			t.Errorf("read typed request: %v", err)
			return
		}
		if outbound.Type != agentmgr.MsgServiceList {
			t.Errorf("message type=%q want %q", outbound.Type, agentmgr.MsgServiceList)
			return
		}
		var request agentmgr.ServiceListData
		if err := json.Unmarshal(outbound.Data, &request); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		data, _ := json.Marshal(agentmgr.ServiceListedData{
			RequestID: request.RequestID,
			Services:  []agentmgr.ServiceInfo{{Name: "sshd", ActiveState: "active"}},
		})
		sut.processAgentServiceListed(&agentmgr.AgentConn{AssetID: "node-svc"}, agentmgr.Message{
			Type: agentmgr.MsgServiceListed,
			ID:   request.RequestID,
			Data: data,
		})
	}()

	value, err := sut.mcpListServices()(context.Background(), "node-svc")
	<-done
	if err != nil {
		t.Fatalf("MCP service list: %v", err)
	}
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("result type=%T", value)
	}
	services, ok := result["services"].([]any)
	if !ok || len(services) != 1 {
		t.Fatalf("services=%#v", result["services"])
	}
}

func TestMCPHTTPRejectsOversizedRequestBeforeParsing(t *testing.T) {
	sut := newTestAPIServer(t)
	body := bytes.Repeat([]byte("x"), maxMCPRequestBodyBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	sut.handleMCP()(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMCPMetricsOverviewUsesProductionStoresAndAllowlist(t *testing.T) {
	sut := newTestAPIServer(t)
	for _, assetID := range []string{"allowed", "hidden"} {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
			AssetID: assetID, Name: assetID, Status: "online", Platform: "linux",
		}); err != nil {
			t.Fatal(err)
		}
	}
	ctx := contextWithAllowedAssets(context.Background(), []string{"allowed"})
	result, err := sut.mcpMetricsOverview()(ctx)
	if err != nil {
		t.Fatal(err)
	}
	entries, ok := result["assets"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("metrics assets=%#v", result["assets"])
	}
	entry, _ := entries[0].(map[string]any)
	if entry["asset_id"] != "allowed" {
		t.Fatalf("metrics leaked or omitted asset: %#v", entry)
	}
}

func TestMCPWakeAssetSendsMagicPacketThroughProductionHandler(t *testing.T) {
	sut := newTestAPIServer(t)
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID: "sleepy", Status: "offline", Metadata: map[string]string{"mac_address": "aa:bb:cc:dd:ee:ff"},
	}); err != nil {
		t.Fatal(err)
	}
	originalSend := respkg.SendWakeOnLAN
	t.Cleanup(func() { respkg.SendWakeOnLAN = originalSend })
	var gotMAC string
	respkg.SendWakeOnLAN = func(mac net.HardwareAddr, _ string) error {
		gotMAC = mac.String()
		return nil
	}
	ctx := contextWithPrincipal(context.Background(), "owner", "admin")
	result, err := sut.mcpWakeAsset()(ctx, "sleepy")
	if err != nil {
		t.Fatal(err)
	}
	if gotMAC != "aa:bb:cc:dd:ee:ff" || result["status"] != "sent" {
		t.Fatalf("MAC=%q result=%#v", gotMAC, result)
	}
}

func TestMCPMutationPolicyRateLimitsByPrincipalAndTarget(t *testing.T) {
	sut := newTestAPIServer(t)
	ctx := contextWithPrincipal(context.Background(), "operator", "admin")
	authorize := sut.mcpAuthorizeMutation()
	for index := 0; index < 30; index++ {
		if err := authorize(ctx, "exec", "srv1"); err != nil {
			t.Fatalf("request %d unexpectedly denied: %v", index+1, err)
		}
	}
	if err := authorize(ctx, "exec", "srv1"); err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("31st request error=%v", err)
	}
	// A different target receives an independent bucket.
	if err := authorize(ctx, "exec", "srv2"); err != nil {
		t.Fatalf("different target unexpectedly denied: %v", err)
	}
}

func TestMCPMutationPolicyRejectsMissingAndViewerPrincipalsBeforeTargetResolution(t *testing.T) {
	sut := newTestAPIServer(t)
	// Leaving the Docker coordinator unset proves that role enforcement happens
	// before an indirect target is resolved or any mutation dependency is used.
	authorize := sut.mcpAuthorizeMutation()

	if err := authorize(context.Background(), "docker_container_restart", "hidden-container"); err == nil || !strings.Contains(err.Error(), "principal") {
		t.Fatalf("missing principal error=%v", err)
	}
	viewerCtx := contextWithPrincipal(context.Background(), "viewer-user", "viewer")
	if err := authorize(viewerCtx, "docker_container_restart", "hidden-container"); err == nil || !strings.Contains(err.Error(), "operator role") {
		t.Fatalf("viewer mutation error=%v", err)
	}
}

func TestMCPDockerAuthorizationFiltersHostsAndHidesRestrictedContainerExistence(t *testing.T) {
	coord := dockerconnector.NewCoordinator(nil)
	coord.HandleDiscovery("host-allowed", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "host-allowed",
		Containers: []agentmgr.DockerContainerInfo{{
			ID: "allowed123456789", Name: "allowed-container", State: "running",
		}},
	}))
	coord.HandleDiscovery("host-hidden", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "host-hidden",
		Containers: []agentmgr.DockerContainerInfo{{
			ID: "hidden123456789", Name: "hidden-container-sentinel", State: "running",
		}},
	}))

	sut := newTestAPIServer(t)
	sut.dockerCoordinator = coord
	ctx := contextWithPrincipal(context.Background(), "apikey:restricted", "operator")
	ctx = contextWithScopes(ctx, []string{"*"})
	ctx = contextWithAllowedAssets(ctx, []string{"host-allowed"})

	mcpServer := sut.buildMCPServer()
	hostsResult, err := mcpServer.GetTool("docker_hosts").Handler(ctx, mcpBridgeToolRequest(nil))
	if err != nil || hostsResult == nil || hostsResult.IsError {
		t.Fatalf("restricted docker host list failed: result=%#v err=%v", hostsResult, err)
	}
	hostsText := mcpBridgeToolResultText(t, hostsResult)
	if !strings.Contains(hostsText, "host-allowed") || strings.Contains(hostsText, "host-hidden") || strings.Contains(hostsText, "hidden-container-sentinel") {
		t.Fatalf("restricted Docker host list leaked or omitted a host: %s", hostsText)
	}

	containersResult, err := mcpServer.GetTool("docker_containers").Handler(ctx, mcpBridgeToolRequest(map[string]any{"host_id": "host-hidden"}))
	if err != nil || containersResult == nil || !containersResult.IsError {
		t.Fatalf("restricted Docker container list was not denied: result=%#v err=%v", containersResult, err)
	}
	if strings.Contains(mcpBridgeToolResultText(t, containersResult), "hidden-container-sentinel") {
		t.Fatalf("restricted Docker container list leaked a container: %s", mcpBridgeToolResultText(t, containersResult))
	}

	authorize := sut.mcpAuthorizeMutation()
	hiddenErr := authorize(ctx, "docker_container_restart", "hidden-container-sentinel")
	missingErr := authorize(ctx, "docker_container_restart", "missing-container")
	if hiddenErr == nil || missingErr == nil || hiddenErr.Error() != missingErr.Error() {
		t.Fatalf("restricted restart disclosed target existence: hidden=%v missing=%v", hiddenErr, missingErr)
	}
	if !strings.Contains(hiddenErr.Error(), "outside allowed assets") || strings.Contains(hiddenErr.Error(), "hidden-container") {
		t.Fatalf("restricted restart returned unsafe error: %v", hiddenErr)
	}
	if err := authorize(ctx, "docker_container_restart", "allowed-container"); err != nil {
		t.Fatalf("allowed Docker container did not pass MCP authorization: %v", err)
	}
}

func TestMCPAuditRedactsUnexpectedDetails(t *testing.T) {
	sut := newTestAPIServer(t)
	ctx := contextWithPrincipal(context.Background(), "operator", "admin")
	sut.mcpAuditMutation()(ctx, "exec", "srv1", "succeeded", "", map[string]any{
		"command_bytes": 8,
		"command":       "secret command",
		"output":        "secret output",
	})
	events, err := sut.auditStore.List(10, 0)
	if err != nil || len(events) != 1 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
	event := events[0]
	if event.Type != "mcp.tool" || event.ActorID != "operator" || event.Details["command_bytes"] != 8 {
		t.Fatalf("event=%#v", event)
	}
	if _, ok := event.Details["command"]; ok {
		t.Fatalf("audit leaked command: %#v", event.Details)
	}
	if _, ok := event.Details["output"]; ok {
		t.Fatalf("audit leaked output: %#v", event.Details)
	}
}

func TestMCPDockerStatsResolvesNameAndBindsHost(t *testing.T) {
	coord := dockerconnector.NewCoordinator(nil)
	coord.HandleDiscovery("agent-01", makeDockerDiscovery(agentmgr.DockerDiscoveryData{
		HostID: "agent-01",
		Containers: []agentmgr.DockerContainerInfo{{
			ID: "abc123def456789", Name: "nginx", State: "running",
		}},
	}))
	statsData, _ := json.Marshal(agentmgr.DockerStatsData{
		HostID: "agent-01",
		Containers: []agentmgr.DockerContainerStats{{
			ID: "abc123def456789", CPUPercent: 12.5,
		}},
	})
	coord.HandleStats("agent-01", agentmgr.Message{Type: agentmgr.MsgDockerStats, Data: statsData})
	sut := newTestAPIServer(t)
	sut.dockerCoordinator = coord
	result, err := sut.mcpDockerContainerStats()(context.Background(), "agent-01", "nginx")
	if err != nil {
		t.Fatal(err)
	}
	if result["available"] != true || result["cpu_percent"] != 12.5 {
		t.Fatalf("stats=%#v", result)
	}
	if _, err := sut.mcpDockerContainerStats()(context.Background(), "agent-02", "nginx"); err == nil {
		t.Fatal("cross-host container reference should be rejected")
	}
}

func TestMCPAlertReadAndAcknowledgeEnforceCurrentAssetAllowlist(t *testing.T) {
	sut := newTestAPIServer(t)
	allowedRule, err := sut.alertStore.CreateAlertRule(alerts.CreateRuleRequest{
		Name: "allowed", Targets: []alerts.RuleTargetInput{{AssetID: "asset-a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	secretRule, err := sut.alertStore.CreateAlertRule(alerts.CreateRuleRequest{
		Name: "secret", Targets: []alerts.RuleTargetInput{{AssetID: "asset-b"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	allowedInstance, err := sut.alertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{
		RuleID: allowedRule.ID, Severity: alerts.SeverityHigh,
	})
	if err != nil {
		t.Fatal(err)
	}
	secretInstance, err := sut.alertInstanceStore.CreateAlertInstance(alerts.CreateInstanceRequest{
		RuleID: secretRule.ID, Severity: alerts.SeverityHigh,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sut.alertInstanceStore.UpdateAlertInstanceStatus(allowedInstance.ID, alerts.InstanceStatusFiring); err != nil {
		t.Fatal(err)
	}
	if _, err := sut.alertInstanceStore.UpdateAlertInstanceStatus(secretInstance.ID, alerts.InstanceStatusFiring); err != nil {
		t.Fatal(err)
	}

	ctx := contextWithAllowedAssets(context.Background(), []string{"asset-a"})
	instances, err := sut.mcpListAlerts()(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 1 || instances[0]["id"] != allowedInstance.ID {
		t.Fatalf("restricted alert list leaked or omitted an instance: %#v", instances)
	}
	alertResource := sut.buildMCPServer().ListResources()["labtether://alerts/active"]
	alertContents, err := alertResource.Handler(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "labtether://alerts/active"},
	})
	if err != nil || len(alertContents) != 1 {
		t.Fatalf("restricted alert resource failed: contents=%#v err=%v", alertContents, err)
	}
	alertText := mcpBridgeResourceText(t, alertContents[0])
	if !strings.Contains(alertText, allowedInstance.ID) || strings.Contains(alertText, secretInstance.ID) {
		t.Fatalf("restricted alert resource leaked or omitted an instance: %s", alertText)
	}
	if err := sut.mcpAcknowledgeAlert()(ctx, secretInstance.ID); err == nil {
		t.Fatal("restricted alert acknowledgement should fail closed")
	}
	unchanged, ok, err := sut.alertInstanceStore.GetAlertInstance(secretInstance.ID)
	if err != nil || !ok || unchanged.Status != alerts.InstanceStatusFiring {
		t.Fatalf("forbidden acknowledgement reached store: instance=%#v ok=%v err=%v", unchanged, ok, err)
	}
	if err := sut.mcpAcknowledgeAlert()(ctx, allowedInstance.ID); err != nil {
		t.Fatalf("allowed acknowledgement failed: %v", err)
	}
}

func TestMCPRestrictedOperationalCollectionsFilterEveryAssetReference(t *testing.T) {
	sut := newTestAPIServer(t)
	actorID := "apikey:restricted"
	ctx := contextWithPrincipal(context.Background(), actorID, "operator")
	ctx = contextWithAllowedAssets(ctx, []string{"asset-a", "asset-c"})

	parent, err := sut.groupStore.CreateGroup(groups.CreateRequest{Name: "mixed parent", Slug: "mixed-parent"})
	if err != nil {
		t.Fatal(err)
	}
	allowedGroup, err := sut.groupStore.CreateGroup(groups.CreateRequest{Name: "allowed", Slug: "allowed", ParentGroupID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	secretGroup, err := sut.groupStore.CreateGroup(groups.CreateRequest{Name: "secret", Slug: "secret", ParentGroupID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, heartbeat := range []assets.HeartbeatRequest{
		{AssetID: "asset-a", Name: "asset-a", GroupID: allowedGroup.ID, Status: "online"},
		{AssetID: "asset-c", Name: "asset-c", GroupID: allowedGroup.ID, Status: "online"},
		{AssetID: "asset-b", Name: "asset-b", GroupID: secretGroup.ID, Status: "online"},
	} {
		if _, err := sut.assetStore.UpsertAssetHeartbeat(heartbeat); err != nil {
			t.Fatal(err)
		}
	}
	groupList, err := sut.mcpListGroups()(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groupList) != 1 || groupList[0]["id"] != allowedGroup.ID {
		t.Fatalf("restricted group list leaked an inaccessible group or parent: %#v", groupList)
	}
	if parentValue, parentPresent := groupList[0]["parent_group_id"]; parentPresent && parentValue != "" {
		t.Fatalf("restricted group list leaked an inaccessible parent: %#v", groupList)
	}
	groupResource := sut.buildMCPServer().ListResources()["labtether://groups"]
	groupContents, err := groupResource.Handler(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "labtether://groups"},
	})
	if err != nil || len(groupContents) != 1 {
		t.Fatalf("restricted group resource failed: contents=%#v err=%v", groupContents, err)
	}
	groupText := mcpBridgeResourceText(t, groupContents[0])
	if !strings.Contains(groupText, allowedGroup.ID) || strings.Contains(groupText, secretGroup.ID) || strings.Contains(groupText, parent.ID) {
		t.Fatalf("restricted group resource leaked or omitted a group: %s", groupText)
	}

	for _, task := range []schedules.ScheduledTask{
		{ID: "schedule-allowed", Name: "allowed", Targets: []string{"asset-a"}, CreatedBy: actorID, CreatedAt: time.Now().UTC()},
		{ID: "schedule-secret", Name: "secret", Targets: []string{"asset-b"}, CreatedBy: actorID, CreatedAt: time.Now().UTC()},
	} {
		if err := sut.scheduleStore.CreateScheduledTask(context.Background(), task); err != nil {
			t.Fatal(err)
		}
	}
	scheduleList, err := sut.mcpListSchedules()(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(scheduleList) != 1 || scheduleList[0]["id"] != "schedule-allowed" {
		t.Fatalf("restricted schedule list leaked or omitted a task: %#v", scheduleList)
	}

	for _, action := range []savedactions.SavedAction{
		{ID: "action-allowed", Name: "allowed", CreatedBy: actorID, Steps: []savedactions.ActionStep{{Name: "one", Target: "asset-c", Command: "uptime"}}},
		{ID: "action-secret", Name: "secret", CreatedBy: actorID, Steps: []savedactions.ActionStep{{Name: "one", Target: "asset-b", Command: "secret-command"}}},
	} {
		if err := sut.savedActionStore.CreateSavedAction(context.Background(), action); err != nil {
			t.Fatal(err)
		}
	}
	actionList, err := sut.mcpListSavedActions()(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(actionList) != 1 || actionList[0]["id"] != "action-allowed" {
		t.Fatalf("restricted saved action list leaked or omitted an action: %#v", actionList)
	}

	allowedPlan, err := sut.updateStore.CreateUpdatePlan(updates.CreatePlanRequest{Name: "allowed", Targets: []string{"asset-a", "asset-c"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sut.updateStore.CreateUpdatePlan(updates.CreatePlanRequest{Name: "secret", Targets: []string{"asset-b"}}); err != nil {
		t.Fatal(err)
	}
	planList, err := sut.mcpListUpdatePlans()(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(planList) != 1 || planList[0]["id"] != allowedPlan.ID {
		t.Fatalf("restricted update plan list leaked or omitted a plan: %#v", planList)
	}

	edgeStore := newMockEdgeStore()
	sut.edgeStore = edgeStore
	allowedEdge, err := edgeStore.CreateEdge(edges.CreateEdgeRequest{SourceAssetID: "asset-a", TargetAssetID: "asset-c"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := edgeStore.CreateEdge(edges.CreateEdgeRequest{SourceAssetID: "asset-a", TargetAssetID: "asset-b"}); err != nil {
		t.Fatal(err)
	}
	edgeList, err := sut.mcpGetEdgesForAsset()(ctx, "asset-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(edgeList) != 1 || edgeList[0]["id"] != allowedEdge.ID {
		t.Fatalf("restricted topology list leaked or omitted an edge: %#v", edgeList)
	}
}

func mcpBridgeToolRequest(arguments map[string]any) mcp.CallToolRequest {
	request := mcp.CallToolRequest{}
	request.Params.Arguments = arguments
	return request
}

func mcpBridgeToolResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		t.Fatal("expected MCP tool text content")
	}
	content, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("MCP tool content type=%T", result.Content[0])
	}
	return content.Text
}

func mcpBridgeResourceText(t *testing.T, content mcp.ResourceContents) string {
	t.Helper()
	text, ok := content.(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("MCP resource content type=%T", content)
	}
	return text.Text
}
