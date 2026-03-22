package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/mark3labs/mcp-go/server"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/mcpserver"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
)

func (s *apiServer) buildMCPServer() *server.MCPServer {
	deps := &mcpserver.Deps{
		AssetStore: s.assetStore,
		AgentMgr:   s.agentMgr,
		ExecuteViaAgent: func(job terminal.CommandJob) terminal.CommandResult {
			return s.executeViaAgent(job)
		},
		GetScopes:        func(ctx context.Context) []string { return scopesFromContext(ctx) },
		GetAllowedAssets: func(ctx context.Context) []string { return allowedAssetsFromContext(ctx) },
		GetActorID:       func(ctx context.Context) string { return principalActorID(ctx) },

		// Docker — wired when the coordinator is present.
		ListDockerHosts:        s.mcpListDockerHosts(),
		ListDockerContainers:   s.mcpListDockerContainers(),
		RestartDockerContainer: s.mcpRestartDockerContainer(),

		// Alerts — wired when the alertInstanceStore is present.
		ListAlerts:       s.mcpListAlerts(),
		AcknowledgeAlert: s.mcpAcknowledgeAlert(),

		// Groups — wired when the groupStore is present.
		ListGroups: s.mcpListGroups(),

		// Metrics overview — complex query, not wired yet; returns "not configured".
		MetricsOverview: nil,

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

func (s *apiServer) handleMCP() http.HandlerFunc {
	mcpSrv := s.buildMCPServer()
	httpSrv := server.NewStreamableHTTPServer(mcpSrv)
	return func(w http.ResponseWriter, r *http.Request) {
		httpSrv.ServeHTTP(w, r)
	}
}

// --- Dependency constructors ---

func (s *apiServer) mcpListDockerHosts() func() ([]map[string]any, error) {
	if s.dockerCoordinator == nil {
		return nil
	}
	return func() ([]map[string]any, error) {
		hosts := s.dockerCoordinator.ListHosts()
		out := make([]map[string]any, 0, len(hosts))
		for _, h := range hosts {
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

func (s *apiServer) mcpListDockerContainers() func(hostID string) ([]map[string]any, error) {
	if s.dockerCoordinator == nil {
		return nil
	}
	return func(hostID string) ([]map[string]any, error) {
		host, ok := s.dockerCoordinator.GetHost(hostID)
		if !ok {
			// Fall back to normalized lookup.
			host, ok = s.dockerCoordinator.GetHostByNormalizedID(hostID)
		}
		if !ok {
			return nil, nil
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

func (s *apiServer) mcpRestartDockerContainer() func(containerID string) error {
	if s.dockerCoordinator == nil {
		return nil
	}
	return func(containerID string) error {
		_, err := s.dockerCoordinator.ExecuteAction(
			context.Background(),
			"container.restart",
			connectorsdk.ActionRequest{TargetID: containerID},
		)
		return err
	}
}

func (s *apiServer) mcpListAlerts() func() ([]map[string]any, error) {
	if s.alertInstanceStore == nil {
		return nil
	}
	return func() ([]map[string]any, error) {
		instances, err := s.alertInstanceStore.ListAlertInstances(persistence.AlertInstanceFilter{
			Status: "firing",
			Limit:  200,
		})
		if err != nil {
			return nil, err
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

func (s *apiServer) mcpAcknowledgeAlert() func(alertID string) error {
	if s.alertInstanceStore == nil {
		return nil
	}
	return func(alertID string) error {
		_, err := s.alertInstanceStore.UpdateAlertInstanceStatus(alertID, "acknowledged")
		return err
	}
}

func (s *apiServer) mcpListGroups() func() ([]map[string]any, error) {
	if s.groupStore == nil {
		return nil
	}
	return func() ([]map[string]any, error) {
		grps, err := s.groupStore.ListGroups()
		if err != nil {
			return nil, err
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
		tasks, err := s.scheduleStore.ListScheduledTasks(ctx)
		if err != nil {
			return nil, err
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
		actions, err := s.savedActionStore.ListSavedActions(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(actions))
		for _, a := range actions {
			b, err := json.Marshal(a)
			if err != nil {
				log.Printf("mcp: mcpListSavedActions: marshal skip: %v", err)
				continue
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				log.Printf("mcp: mcpListSavedActions: unmarshal skip: %v", err)
				continue
			}
			out = append(out, m)
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
		out := make([]map[string]any, 0, len(descriptors))
		for _, desc := range descriptors {
			connector, ok := s.connectorRegistry.Get(desc.ID)
			if !ok {
				continue
			}
			health, err := connector.TestConnection(ctx)
			entry := map[string]any{
				"id":           desc.ID,
				"display_name": desc.DisplayName,
				"capabilities": desc.Capabilities,
			}
			if err != nil {
				entry["status"] = "error"
				entry["message"] = err.Error()
			} else {
				entry["status"] = health.Status
				entry["message"] = health.Message
			}
			out = append(out, entry)
		}
		return out, nil
	}
}
