package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/mcpserver"
	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
	"github.com/mark3labs/mcp-go/server"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxMCPRequestBodyBytes = 128 * 1024

func (s *apiServer) buildMCPServer() *server.MCPServer {
	deps := &mcpserver.Deps{
		AssetStore: s.assetStore,
		AgentMgr:   s.agentMgr,
		ExecuteViaAgent: func(job terminal.CommandJob) terminal.CommandResult {
			return s.executeViaAgent(job)
		},
		ExecutePowerAction:    s.mcpExecutePowerAction(),
		GetScopes:             func(ctx context.Context) []string { return scopesFromContext(ctx) },
		GetAllowedAssets:      func(ctx context.Context) []string { return allowedAssetsFromContext(ctx) },
		GetActorID:            func(ctx context.Context) string { return principalActorID(ctx) },
		AuthorizeMutation:     s.mcpAuthorizeMutation(),
		EvaluateCommandPolicy: s.evaluateStructuredCommandPolicy,
		AuditMutation:         s.mcpAuditMutation(),

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
			p.Metadata = securityruntime.RedactURLUserinfoValues(p.Metadata)
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
