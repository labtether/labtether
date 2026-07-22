package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/apikeys"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/mcpserver"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/schedules"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

const maxMCPRequestBodyBytes = 128 * 1024

func (s *apiServer) buildMCPServer() *server.MCPServer {
	deps := &mcpserver.Deps{
		AssetStore: s.assetStore,
		AgentMgr:   s.agentMgr,
		ExecuteViaAgent: func(job terminal.CommandJob) terminal.CommandResult {
			return s.executeViaAgent(job)
		},
		ExecutePowerAction: s.mcpExecutePowerAction(),
		GetScopes:          func(ctx context.Context) []string { return scopesFromContext(ctx) },
		GetAllowedAssets:   func(ctx context.Context) []string { return allowedAssetsFromContext(ctx) },
		GetActorID:         func(ctx context.Context) string { return principalActorID(ctx) },
		AuthorizeMutation:  s.mcpAuthorizeMutation(),
		AuditMutation:      s.mcpAuditMutation(),

		ListServices:   s.mcpListServices(),
		RestartService: s.mcpRestartService(),
		ListFiles:      s.mcpListFiles(),
		ReadFile:       s.mcpReadFile(),
		ListProcesses:  s.mcpListProcesses(),
		ListNetwork:    s.mcpListNetwork(),
		ListDisks:      s.mcpListDisks(),
		ListPackages:   s.mcpListPackages(),

		// Docker — wired when the coordinator is present.
		ListDockerHosts:        s.mcpListDockerHosts(),
		ListDockerContainers:   s.mcpListDockerContainers(),
		RestartDockerContainer: s.mcpRestartDockerContainer(),
		DockerContainerLogs:    s.mcpDockerContainerLogs(),
		DockerContainerStats:   s.mcpDockerContainerStats(),

		// Alerts — wired when the alertInstanceStore is present.
		ListAlerts:       s.mcpListAlerts(),
		AcknowledgeAlert: s.mcpAcknowledgeAlert(),

		// Groups — wired when the groupStore is present.
		ListGroups: s.mcpListGroups(),

		MetricsOverview: s.mcpMetricsOverview(),
		WakeAsset:       s.mcpWakeAsset(),

		// Operational stores.
		ListSchedules:          s.mcpListSchedules(),
		ListWebhooks:           s.mcpListWebhooks(),
		ListSavedActions:       s.mcpListSavedActions(),
		ListCredentialProfiles: s.mcpListCredentialProfiles(),
		GetEdgesForAsset:       s.mcpGetEdgesForAsset(),
		ListUpdatePlans:        s.mcpListUpdatePlans(),

		// Connector health — wired when the registry is present.
		ConnectorsHealth: s.mcpConnectorsHealth(),
	}
	return mcpserver.NewServer(deps)
}

func (s *apiServer) mcpExecutePowerAction() func(context.Context, string, string) (string, error) {
	return func(ctx context.Context, assetID, rawAction string) (string, error) {
		action := agentmgr.PowerAction(strings.TrimSpace(rawAction))
		if !action.Valid() {
			return "", fmt.Errorf("unsupported power action")
		}
		result, err := s.ensurePowerCoordinator().Execute(ctx, assetID, action)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s accepted for %s (request %s)", action, assetID, result.RequestID), nil
	}
}

func (s *apiServer) handleMCP() http.HandlerFunc {
	mcpSrv := s.buildMCPServer()
	httpSrv := server.NewStreamableHTTPServer(mcpSrv)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.Body != nil {
			if r.ContentLength > maxMCPRequestBodyBytes {
				servicehttp.WriteError(w, http.StatusRequestEntityTooLarge, "MCP request body exceeds size limit")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxMCPRequestBodyBytes)
		}
		httpSrv.ServeHTTP(w, r)
	}
}

// --- Dependency constructors ---

func (s *apiServer) mcpListDockerHosts() func(context.Context) ([]map[string]any, error) {
	if s.dockerCoordinator == nil {
		return nil
	}
	return func(ctx context.Context) ([]map[string]any, error) {
		hosts := s.dockerCoordinator.ListHosts()
		out := make([]map[string]any, 0, len(hosts))
		allowedAssets := allowedAssetsFromContext(ctx)
		for _, h := range hosts {
			if !apikeys.AssetAllowed(allowedAssets, h.AgentID) {
				continue
			}
			b, err := json.Marshal(h)
			if err != nil {
				log.Printf("mcp: mcpListDockerHosts: marshal skip: %v", err)
				continue
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				log.Printf("mcp: mcpListDockerHosts: unmarshal skip: %v", err)
				continue
			}
			out = append(out, m)
		}
		return out, nil
	}
}

func (s *apiServer) mcpListDockerContainers() func(context.Context, string) ([]map[string]any, error) {
	if s.dockerCoordinator == nil {
		return nil
	}
	return func(ctx context.Context, hostID string) ([]map[string]any, error) {
		host, ok := s.dockerCoordinator.GetHost(hostID)
		if !ok {
			// Fall back to normalized lookup.
			host, ok = s.dockerCoordinator.GetHostByNormalizedID(hostID)
		}
		if !ok {
			return nil, fmt.Errorf("docker host not found: %s", hostID)
		}
		if !apikeys.AssetAllowed(allowedAssetsFromContext(ctx), host.AgentID) {
			return nil, fmt.Errorf("access denied to asset: %s", host.AgentID)
		}
		out := make([]map[string]any, 0, len(host.Containers))
		for _, c := range host.Containers {
			b, err := json.Marshal(c)
			if err != nil {
				log.Printf("mcp: mcpListDockerContainers: marshal skip: %v", err)
				continue
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				log.Printf("mcp: mcpListDockerContainers: unmarshal skip: %v", err)
				continue
			}
			out = append(out, m)
		}
		return out, nil
	}
}

func (s *apiServer) mcpRestartDockerContainer() func(context.Context, string) error {
	if s.dockerCoordinator == nil {
		return nil
	}
	return func(ctx context.Context, containerID string) error {
		resolved, err := s.mcpResolveDockerContainer("", containerID)
		if err != nil {
			return err
		}
		if !apikeys.AssetAllowed(allowedAssetsFromContext(ctx), resolved.host.AgentID) {
			return fmt.Errorf("access denied to asset: %s", resolved.host.AgentID)
		}
		guardrails, err := s.ensureGroupFeaturesDeps().EvaluateAssetGuardrails(resolved.host.AgentID, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("failed to evaluate maintenance windows")
		}
		if guardrails.BlockActions {
			return fmt.Errorf("actions are blocked by active maintenance windows")
		}
		result, err := s.dockerCoordinator.ExecuteAction(
			ctx,
			"container.restart",
			connectorsdk.ActionRequest{TargetID: resolved.canonical},
		)
		if err != nil {
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(result.Status), "succeeded") {
			message := strings.TrimSpace(result.Message)
			if message == "" {
				message = "docker container restart failed"
			}
			return fmt.Errorf("%s", message)
		}
		return nil
	}
}

func (s *apiServer) mcpListAlerts() func(context.Context) ([]map[string]any, error) {
	if s.alertInstanceStore == nil {
		return nil
	}
	return func(ctx context.Context) ([]map[string]any, error) {
		instances, err := s.alertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
			Status: "firing",
			Limit:  200,
		})
		if err != nil {
			return nil, err
		}
		instances, err = s.ensureAlertingDeps().FilterAlertInstancesForAccess(ctx, instances)
		if err != nil {
			return nil, fmt.Errorf("unable to prove alert asset scope: %w", err)
		}
		out := make([]map[string]any, 0, len(instances))
		for _, inst := range instances {
			b, err := json.Marshal(inst)
			if err != nil {
				log.Printf("mcp: mcpListAlerts: marshal skip: %v", err)
				continue
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				log.Printf("mcp: mcpListAlerts: unmarshal skip: %v", err)
				continue
			}
			out = append(out, m)
		}
		return out, nil
	}
}

func (s *apiServer) mcpAcknowledgeAlert() func(context.Context, string) error {
	if s.alertInstanceStore == nil {
		return nil
	}
	return func(ctx context.Context, alertID string) error {
		instance, found, err := s.alertInstanceStore.GetAlertInstance(alertID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("alert is unavailable or outside allowed assets")
		}
		allowed, err := s.ensureAlertingDeps().AlertInstanceAllowedForAccess(ctx, instance)
		if err != nil {
			return fmt.Errorf("unable to prove alert asset scope: %w", err)
		}
		if !allowed {
			return fmt.Errorf("alert is unavailable or outside allowed assets")
		}
		_, err = s.alertInstanceStore.UpdateAlertInstanceStatus(alertID, "acknowledged")
		return err
	}
}

func (s *apiServer) mcpListGroups() func(context.Context) ([]map[string]any, error) {
	if s.groupStore == nil {
		return nil
	}
	return func(ctx context.Context) ([]map[string]any, error) {
		grps, err := s.groupStore.ListGroups()
		if err != nil {
			return nil, err
		}
		if shared.HasAssetRestriction(ctx) {
			if s.assetStore == nil {
				return nil, fmt.Errorf("asset authorization store unavailable")
			}
			assetList, err := s.assetStore.ListAssets()
			if err != nil {
				return nil, fmt.Errorf("load assets for group authorization: %w", err)
			}
			accessible := shared.AccessibleGroupIDs(ctx, grps, assetList)
			grps = shared.FilterGroupsByAssetAccess(ctx, grps, assetList)
			for i := range grps {
				if _, parentAllowed := accessible[strings.TrimSpace(grps[i].ParentGroupID)]; !parentAllowed {
					grps[i].ParentGroupID = ""
				}
			}
		}
		out := make([]map[string]any, 0, len(grps))
		for _, g := range grps {
			b, err := json.Marshal(g)
			if err != nil {
				log.Printf("mcp: mcpListGroups: marshal skip: %v", err)
				continue
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				log.Printf("mcp: mcpListGroups: unmarshal skip: %v", err)
				continue
			}
			out = append(out, m)
		}
		return out, nil
	}
}

func (s *apiServer) mcpListSchedules() func(ctx context.Context) ([]map[string]any, error) {
	if s.scheduleStore == nil {
		return nil
	}
	return func(ctx context.Context) ([]map[string]any, error) {
		limit := schedules.MaxScheduledTaskPageSize
		if shared.HasAssetRestriction(ctx) {
			limit = schedules.MaxScheduledTasksGlobal + 1
		}
		tasks, total, err := s.scheduleStore.ListScheduledTasks(ctx, limit, 0)
		if err != nil {
			return nil, err
		}
		if shared.HasAssetRestriction(ctx) {
			if total > schedules.MaxScheduledTasksGlobal {
				return nil, schedules.ErrScheduledTaskCapacityExceeded
			}
			tasks, err = s.ensureSchedulesDeps().FilterScheduledTasksForAccess(ctx, tasks)
			if err != nil {
				return nil, fmt.Errorf("unable to prove schedule asset scope: %w", err)
			}
			total = len(tasks)
		}
		if total > schedules.MaxScheduledTaskPageSize {
			return nil, fmt.Errorf("schedule list contains %d definitions; use the paginated API (MCP limit %d)", total, schedules.MaxScheduledTaskPageSize)
		}
		out := make([]map[string]any, 0, len(tasks))
		for _, t := range tasks {
			b, err := json.Marshal(t)
			if err != nil {
				log.Printf("mcp: mcpListSchedules: marshal skip: %v", err)
				continue
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				log.Printf("mcp: mcpListSchedules: unmarshal skip: %v", err)
				continue
			}
			out = append(out, m)
		}
		return out, nil
	}
}

func (s *apiServer) mcpListWebhooks() func(ctx context.Context) ([]map[string]any, error) {
	if s.webhookStore == nil {
		return nil
	}
	return func(ctx context.Context) ([]map[string]any, error) {
		whs, err := s.webhookStore.ListWebhooks(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(whs))
		for _, wh := range whs {
			b, err := json.Marshal(wh)
			if err != nil {
				log.Printf("mcp: mcpListWebhooks: marshal skip: %v", err)
				continue
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				log.Printf("mcp: mcpListWebhooks: unmarshal skip: %v", err)
				continue
			}
			out = append(out, m)
		}
		return out, nil
	}
}

func (s *apiServer) mcpListSavedActions() func(ctx context.Context) ([]map[string]any, error) {
	if s.savedActionStore == nil {
		return nil
	}
	return func(ctx context.Context) ([]map[string]any, error) {
		actions, _, err := s.savedActionStore.ListSavedActions(ctx, apiv2.PrincipalActorID(ctx), 200, 0)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(actions))
		for _, a := range actions {
			targets := make([]string, 0, len(a.Steps))
			for _, step := range a.Steps {
				targets = append(targets, step.Target)
			}
			if !shared.AllAssetsAllowed(ctx, targets...) {
				continue
			}
			steps := make([]map[string]any, 0, len(a.Steps))
			for _, step := range a.Steps {
				steps = append(steps, map[string]any{
					"name":    step.Name,
					"command": step.Command,
					"target":  step.Target,
				})
			}
			out = append(out, map[string]any{
				"id":          a.ID,
				"name":        a.Name,
				"description": a.Description,
				"steps":       steps,
				"created_by":  a.CreatedBy,
				"created_at":  a.CreatedAt,
			})
		}
		return out, nil
	}
}

func (s *apiServer) mcpListCredentialProfiles() func(ctx context.Context) ([]map[string]any, error) {
	if s.credentialStore == nil {
		return nil
	}
	return func(ctx context.Context) ([]map[string]any, error) {
		// SecretCiphertext and PassphraseCiphertext carry json:"-" tags on the
		// credentials.Profile struct, so json.Marshal naturally omits them.
		profiles, err := s.credentialStore.ListCredentialProfiles(200)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(profiles))
		for _, p := range profiles {
			b, err := json.Marshal(p)
			if err != nil {
				log.Printf("mcp: mcpListCredentialProfiles: marshal skip: %v", err)
				continue
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				log.Printf("mcp: mcpListCredentialProfiles: unmarshal skip: %v", err)
				continue
			}
			out = append(out, m)
		}
		return out, nil
	}
}

func (s *apiServer) mcpGetEdgesForAsset() func(ctx context.Context, assetID string) ([]map[string]any, error) {
	if s.edgeStore == nil {
		return nil
	}
	return func(ctx context.Context, assetID string) ([]map[string]any, error) {
		edgeList, err := s.edgeStore.ListEdgesByAsset(assetID, 500)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(edgeList))
		for _, e := range edgeList {
			if !shared.AllAssetsAllowed(ctx, e.SourceAssetID, e.TargetAssetID) {
				continue
			}
			b, err := json.Marshal(e)
			if err != nil {
				log.Printf("mcp: mcpGetEdgesForAsset: marshal skip: %v", err)
				continue
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				log.Printf("mcp: mcpGetEdgesForAsset: unmarshal skip: %v", err)
				continue
			}
			out = append(out, m)
		}
		return out, nil
	}
}

func (s *apiServer) mcpListUpdatePlans() func(ctx context.Context) ([]map[string]any, error) {
	if s.updateStore == nil {
		return nil
	}
	return func(ctx context.Context) ([]map[string]any, error) {
		plans, err := s.updateStore.ListUpdatePlans(200)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(plans))
		for _, p := range plans {
			if !shared.AllAssetsAllowed(ctx, p.Targets...) {
				continue
			}
			b, err := json.Marshal(p)
			if err != nil {
				log.Printf("mcp: mcpListUpdatePlans: marshal skip: %v", err)
				continue
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				log.Printf("mcp: mcpListUpdatePlans: unmarshal skip: %v", err)
				continue
			}
			out = append(out, m)
		}
		return out, nil
	}
}

func (s *apiServer) mcpConnectorsHealth() func(ctx context.Context) ([]map[string]any, error) {
	if s.connectorRegistry == nil {
		return nil
	}
	return func(ctx context.Context) ([]map[string]any, error) {
		descriptors := s.connectorRegistry.List()
		if len(descriptors) > 200 {
			return nil, fmt.Errorf("connector inventory exceeds MCP limit")
		}
		type healthResult struct {
			index int
			entry map[string]any
		}
		jobs := make(chan int, len(descriptors))
		results := make(chan healthResult, len(descriptors))
		for index := range descriptors {
			jobs <- index
		}
		close(jobs)
		overallCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		var workers sync.WaitGroup
		for range min(8, len(descriptors)) {
			workers.Add(1)
			go func() {
				defer workers.Done()
				for index := range jobs {
					desc := descriptors[index]
					entry := map[string]any{
						"id":           desc.ID,
						"display_name": desc.DisplayName,
						"capabilities": desc.Capabilities,
					}
					connector, ok := s.connectorRegistry.Get(desc.ID)
					if !ok {
						entry["status"] = "unavailable"
						results <- healthResult{index: index, entry: entry}
						continue
					}
					checkCtx, checkCancel := context.WithTimeout(overallCtx, 5*time.Second)
					health, err := connector.TestConnection(checkCtx)
					checkCancel()
					if err != nil {
						entry["status"] = "error"
						entry["message"] = "connection test failed"
					} else {
						entry["status"] = health.Status
						entry["message"] = safeMCPConnectorMessage(health.Message)
					}
					results <- healthResult{index: index, entry: entry}
				}
			}()
		}
		outByIndex := make([]map[string]any, len(descriptors))
		for received := 0; received < len(descriptors); received++ {
			select {
			case result := <-results:
				outByIndex[result.index] = result.entry
			case <-overallCtx.Done():
				return nil, fmt.Errorf("connector health checks timed out")
			}
		}
		workers.Wait()
		out := make([]map[string]any, 0, len(outByIndex))
		for _, entry := range outByIndex {
			if entry != nil {
				out = append(out, entry)
			}
		}
		return out, nil
	}
}
