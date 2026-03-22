package resources

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	packageRequestTimeout = 30 * time.Second
	packageActionTimeout  = 10 * time.Minute
)

var validPackageActions = map[string]bool{
	"install": true,
	"remove":  true,
	"upgrade": true,
}

// packageBridge holds the channel for a pending package list request.

// handlePackages dispatches /packages/{assetId} and /packages/{assetId}/{action} requests.
func (d *Deps) HandlePackages(w http.ResponseWriter, r *http.Request) {
	// Extract path after /packages/
	path := strings.TrimPrefix(r.URL.Path, "/packages/")
	if path == "" || path == r.URL.Path {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id required")
		return
	}

	parts := strings.SplitN(path, "/", 2)
	assetID := strings.TrimSpace(parts[0])
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id required")
		return
	}

	action := ""
	if len(parts) > 1 {
		action = strings.TrimSpace(parts[1])
	}

	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(assetID) {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent not connected")
		return
	}

	if action == "" {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		d.handlePackageList(w, r, assetID)
		return
	}

	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	d.handlePackageAction(w, r, assetID, action)
}

func (d *Deps) handlePackageList(w http.ResponseWriter, r *http.Request, assetID string) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	requestID := generateRequestID()

	bridge := &PackageBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: assetID,
	}
	d.PackageBridges.Store(requestID, bridge)
	defer d.PackageBridges.Delete(requestID)

	data, _ := json.Marshal(agentmgr.PackageListData{
		RequestID: requestID,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgPackageList,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var listed agentmgr.PackageListedData
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if listed.Error != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, listed.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, listed)
	case <-time.After(packageRequestTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

func (d *Deps) handlePackageAction(w http.ResponseWriter, r *http.Request, assetID, action string) {
	if !validPackageActions[action] {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid action: must be one of install, remove, or upgrade")
		return
	}

	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	var body struct {
		Packages []string `json:"packages"`
		Package  string   `json:"package"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if single := strings.TrimSpace(body.Package); single != "" {
		body.Packages = append(body.Packages, single)
	}
	cleanedPackages := make([]string, 0, len(body.Packages))
	seen := make(map[string]struct{}, len(body.Packages))
	for _, pkg := range body.Packages {
		trimmed := strings.TrimSpace(pkg)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		cleanedPackages = append(cleanedPackages, trimmed)
	}

	if (action == "install" || action == "remove") && len(cleanedPackages) == 0 {
		servicehttp.WriteError(w, http.StatusBadRequest, "at least one package is required")
		return
	}

	requestID := generateRequestID()
	bridge := &PackageBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: assetID,
	}
	d.PackageBridges.Store(requestID, bridge)
	defer d.PackageBridges.Delete(requestID)

	data, _ := json.Marshal(agentmgr.PackageActionData{
		RequestID: requestID,
		Action:    action,
		Packages:  cleanedPackages,
	})
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgPackageAction,
		ID:   requestID,
		Data: data,
	}); err != nil {
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send request to agent")
		return
	}

	select {
	case msg := <-bridge.Ch:
		var result agentmgr.PackageResultData
		if err := json.Unmarshal(msg.Data, &result); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if result.Error != "" {
			servicehttp.WriteError(w, http.StatusBadRequest, result.Error)
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, result)
	case <-time.After(packageActionTimeout):
		servicehttp.WriteError(w, http.StatusGatewayTimeout, "agent did not respond in time")
	}
}

func (d *Deps) ProcessAgentPackageListed(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.PackageListedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid package listed data: %v", err)
		return
	}
	if raw, ok := d.PackageBridges.Load(data.RequestID); ok {
		bridge, ok := raw.(*PackageBridge)
		if !ok || bridge == nil {
			return
		}
		if conn == nil || (strings.TrimSpace(bridge.ExpectedAssetID) != "" && !strings.EqualFold(strings.TrimSpace(bridge.ExpectedAssetID), strings.TrimSpace(conn.AssetID))) {
			return
		}
		select {
		case bridge.Ch <- msg:
		default:
		}
	}
}

func (d *Deps) ProcessAgentPackageResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var data agentmgr.PackageResultData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid package result data: %v", err)
		return
	}
	if raw, ok := d.PackageBridges.Load(data.RequestID); ok {
		bridge, ok := raw.(*PackageBridge)
		if !ok || bridge == nil {
			return
		}
		if conn == nil || (strings.TrimSpace(bridge.ExpectedAssetID) != "" && !strings.EqualFold(strings.TrimSpace(bridge.ExpectedAssetID), strings.TrimSpace(conn.AssetID))) {
			return
		}
		select {
		case bridge.Ch <- msg:
		default:
		}
	}
}
