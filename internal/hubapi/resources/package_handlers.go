package resources

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

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

const packageUpdatePreviewSummaryMaxBytes = 220

// packageBridge holds the channel for a pending package list request.

// HandlePackages dispatches exact v1 package inventory and action routes.
// Public "update" is retained as an alias while the agent wire keeps the
// package-manager-native canonical action name "upgrade".
func (d *Deps) HandlePackages(w http.ResponseWriter, r *http.Request) {
	// Extract path after /packages/
	path := strings.TrimPrefix(r.URL.Path, "/packages/")
	if path == "" || path == r.URL.Path {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id required")
		return
	}

	parts := strings.Split(path, "/")
	if len(parts) > 2 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown package route")
		return
	}
	assetID := strings.TrimSpace(parts[0])
	if assetID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "asset id required")
		return
	}

	subPath := ""
	if len(parts) > 1 {
		subPath = strings.ToLower(strings.TrimSpace(parts[1]))
	}

	if subPath == "" || subPath == packageInventoryUpgradable {
		if r.Method != http.MethodGet {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if d.AgentMgr == nil || !d.AgentMgr.IsConnected(assetID) {
			servicehttp.WriteError(w, http.StatusBadGateway, "agent not connected")
			return
		}
		inventory := packageInventoryInstalled
		if subPath == packageInventoryUpgradable {
			inventory = packageInventoryUpgradable
		}
		d.handlePackageList(w, r, assetID, inventory)
		return
	}

	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	action := subPath
	if action == "update" {
		action = "upgrade"
	}
	if !validPackageActions[action] {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown package action")
		return
	}
	if d.AgentMgr == nil || !d.AgentMgr.IsConnected(assetID) {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent not connected")
		return
	}
	d.handlePackageAction(w, r, assetID, action)
}

func (d *Deps) handlePackageList(w http.ResponseWriter, r *http.Request, assetID, inventory string) {
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

	request := packageListRequestWire{
		RequestID: requestID,
	}
	if inventory == packageInventoryUpgradable {
		request.Inventory = packageInventoryUpgradable
	}
	data, _ := json.Marshal(request)
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
		var listed packageListedResponseWire
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "invalid agent response")
			return
		}
		if err := validatePackageListedResponse(listed, requestID, inventory); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, err.Error())
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

// PreviewOSPackageUpdatesViaAgent requests the agent's validated upgradable
// package inventory without sending an update.request or applying changes.
// Agents predating the inventory discriminator fail closed because
// validatePackageListedResponse requires them to echo "upgradable".
func (d *Deps) PreviewOSPackageUpdatesViaAgent(requestID, assetID string, timeout time.Duration) agentmgr.CommandResultData {
	requestID = strings.TrimSpace(requestID)
	assetID = strings.TrimSpace(assetID)
	failed := func(message string) agentmgr.CommandResultData {
		return agentmgr.CommandResultData{
			JobID:  requestID,
			Status: "failed",
			Output: strings.TrimSpace(message),
		}
	}

	if requestID == "" || len(requestID) > maxPackageRequestIDBytes {
		return failed("package update preview request id is invalid; no changes applied")
	}
	if assetID == "" {
		return failed("package update preview target is required; no changes applied")
	}
	if d.AgentMgr == nil {
		return failed("agent manager unavailable; no changes applied")
	}
	if d.PackageBridges == nil {
		return failed("package update preview bridge unavailable; no changes applied")
	}
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		return failed("agent not connected; no changes applied")
	}
	if timeout <= 0 {
		timeout = packageRequestTimeout
	}

	bridge := &PackageBridge{
		Ch:              make(chan agentmgr.Message, 1),
		ExpectedAssetID: assetID,
	}
	if _, loaded := d.PackageBridges.LoadOrStore(requestID, bridge); loaded {
		return failed("package update preview request id is already in use; no changes applied")
	}
	defer d.PackageBridges.Delete(requestID)

	data, err := json.Marshal(packageListRequestWire{
		RequestID: requestID,
		Inventory: packageInventoryUpgradable,
	})
	if err != nil {
		return failed("failed to encode package update preview request; no changes applied")
	}
	if err := agentConn.Send(agentmgr.Message{
		Type: agentmgr.MsgPackageList,
		ID:   requestID,
		Data: data,
	}); err != nil {
		return failed("failed to send package update preview request to agent; no changes applied")
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case msg := <-bridge.Ch:
		var listed packageListedResponseWire
		if err := json.Unmarshal(msg.Data, &listed); err != nil {
			return failed("invalid package update preview response; no changes applied")
		}
		if err := validatePackageListedResponse(listed, requestID, packageInventoryUpgradable); err != nil {
			return failed("package update preview failed; no changes applied: " + err.Error())
		}
		if strings.TrimSpace(listed.Error) != "" {
			return failed("package update preview failed; no changes applied: agent reported " + strings.TrimSpace(listed.Error))
		}
		return agentmgr.CommandResultData{
			JobID:  requestID,
			Status: "succeeded",
			Output: summarizeOSPackageUpdatePreview(assetID, listed.Packages),
		}
	case <-timer.C:
		return failed("agent did not respond to package update preview in time; no changes applied")
	}
}

func summarizeOSPackageUpdatePreview(assetID string, packages []packageInfoWire) string {
	if len(packages) == 0 {
		return fmt.Sprintf("dry-run preview: no changes applied on %s; no OS package updates are currently available", assetID)
	}

	const maxDetailedPackages = 3
	detailCount := len(packages)
	if detailCount > maxDetailedPackages {
		detailCount = maxDetailedPackages
	}
	details := make([]string, 0, detailCount+1)
	for _, pkg := range packages[:detailCount] {
		details = append(details, fmt.Sprintf("%s %s -> %s", pkg.Name, pkg.Version, pkg.AvailableVersion))
	}
	if remaining := len(packages) - detailCount; remaining > 0 {
		details = append(details, fmt.Sprintf("and %d more", remaining))
	}
	summary := fmt.Sprintf(
		"dry-run preview: no changes applied on %s; %d OS package update(s) available: %s",
		assetID,
		len(packages),
		strings.Join(details, ", "),
	)
	if len(summary) > packageUpdatePreviewSummaryMaxBytes {
		end := packageUpdatePreviewSummaryMaxBytes - 3
		for end > 0 && !utf8.RuneStart(summary[end]) {
			end--
		}
		return summary[:end] + "..."
	}
	return summary
}

func (d *Deps) handlePackageAction(w http.ResponseWriter, r *http.Request, assetID, action string) {
	if !validPackageActions[action] {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid action: must be one of install, remove, or upgrade")
		return
	}
	if !d.enforceAssetUpdateGuard(w, assetID) {
		return
	}

	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		servicehttp.WriteError(w, http.StatusBadGateway, "agent disconnected")
		return
	}

	var body struct {
		Packages []string `json:"packages"`
		Package  *string  `json:"package"`
	}
	if err := decodeJSONBody(w, r, &body); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	rawPackages := append([]string(nil), body.Packages...)
	if body.Package != nil {
		rawPackages = append(rawPackages, *body.Package)
	}
	cleanedPackages, err := normalizeAndValidatePackageTokens(rawPackages)
	if err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
		return
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
		if err := validatePackageActionResult(result, requestID); err != nil {
			servicehttp.WriteError(w, http.StatusBadGateway, err.Error())
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
	var data packageListedResponseWire
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		log.Printf("agentws: invalid package listed data: %v", err)
		return
	}
	requestID := strings.TrimSpace(data.RequestID)
	if requestID == "" || len(requestID) > maxPackageRequestIDBytes {
		return
	}
	if raw, ok := d.PackageBridges.Load(requestID); ok {
		bridge, ok := raw.(*PackageBridge)
		if !ok || bridge == nil {
			return
		}
		if conn == nil || (strings.TrimSpace(bridge.ExpectedAssetID) != "" && !strings.EqualFold(strings.TrimSpace(bridge.ExpectedAssetID), strings.TrimSpace(conn.AssetID))) {
			return
		}
		if strings.TrimSpace(msg.ID) != "" && strings.TrimSpace(msg.ID) != strings.TrimSpace(data.RequestID) {
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
	requestID := strings.TrimSpace(data.RequestID)
	if requestID == "" || len(requestID) > maxPackageRequestIDBytes {
		return
	}
	if raw, ok := d.PackageBridges.Load(requestID); ok {
		bridge, ok := raw.(*PackageBridge)
		if !ok || bridge == nil {
			return
		}
		if conn == nil || (strings.TrimSpace(bridge.ExpectedAssetID) != "" && !strings.EqualFold(strings.TrimSpace(bridge.ExpectedAssetID), strings.TrimSpace(conn.AssetID))) {
			return
		}
		if strings.TrimSpace(msg.ID) != "" && strings.TrimSpace(msg.ID) != strings.TrimSpace(data.RequestID) {
			return
		}
		select {
		case bridge.Ch <- msg:
		default:
		}
	}
}
