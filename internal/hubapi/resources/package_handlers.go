package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/labtether/labtether/internal/agentcaps"
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
			d.writeAgentNotConnected(w, assetID)
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
		d.writeAgentNotConnected(w, assetID)
		return
	}
	d.handlePackageAction(w, r, assetID, action)
}

func (d *Deps) handlePackageList(w http.ResponseWriter, r *http.Request, assetID, inventory string) {
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		d.writeAgentNotConnected(w, assetID)
		return
	}

	requestID := generateRequestID()
	request := packageListRequestWire{
		RequestID: requestID,
	}
	if inventory == packageInventoryUpgradable {
		request.Inventory = packageInventoryUpgradable
	}
	msg, err := d.requestAgent(d.PackageReplies, agentConn, agentmgr.MsgPackageList, requestID,
		request, assetID, packageRequestTimeout)
	if err != nil {
		d.writeAgentRequestFailure(w, assetID, err)
		return
	}
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
}

// PreviewOSPackageUpdatesViaAgent requests the agent's validated upgradable
// package inventory without sending an update.request or applying changes.
// Agents predating the inventory discriminator fail closed because
// validatePackageListedResponse requires them to echo "upgradable".
func (d *Deps) PreviewOSPackageUpdatesViaAgent(requestID, assetID string, timeout time.Duration) agentmgr.CommandResultData {
	return d.previewOSPackageUpdatesViaAgent(context.Background(), requestID, assetID, timeout, false)
}

func (d *Deps) PreviewOSPackageUpdatesViaAgentContext(ctx context.Context, requestID, assetID string, timeout time.Duration) agentmgr.CommandResultData {
	return d.previewOSPackageUpdatesViaAgent(ctx, requestID, assetID, timeout, true)
}

func (d *Deps) previewOSPackageUpdatesViaAgent(ctx context.Context, requestID, assetID string, timeout time.Duration, privateAttempt bool) agentmgr.CommandResultData {
	requestID = strings.TrimSpace(requestID)
	assetID = strings.TrimSpace(assetID)
	failed := func(message string) agentmgr.CommandResultData {
		return agentmgr.CommandResultData{
			JobID:  requestID,
			Status: "failed",
			Output: strings.TrimSpace(message),
		}
	}

	if err := ctx.Err(); err != nil {
		return failed("package update preview canceled before dispatch; no changes applied: " + err.Error())
	}
	wireID := requestID
	if privateAttempt {
		wireID = "preview_" + generateRequestID()
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
	if d.PackageReplies == nil {
		return failed("package update preview bridge unavailable; no changes applied")
	}
	agentConn, ok := d.AgentMgr.Get(assetID)
	if !ok {
		return failed("agent not connected; no changes applied")
	}
	if !agentcaps.ConnOffers(agentConn, agentmgr.MsgPackageList) {
		return failed(servicehttp.AgentUnsupportedMessage + "; no changes applied")
	}
	if timeout <= 0 {
		timeout = packageRequestTimeout
	}

	msg, err := agentmgr.Request(ctx, d.PackageReplies, agentmgr.Call[packageListRequestWire, agentmgr.Message]{
		Type: agentmgr.MsgPackageList,
		ID:   wireID,
		// The caller's request ID stays held while the preview runs, so a
		// second preview with it is refused. No reply can reach it.
		Reserve:    []string{requestID},
		Payload:    packageListRequestWire{RequestID: wireID, Inventory: packageInventoryUpgradable},
		Send:       func(msg agentmgr.Message) error { return agentConn.SendContext(ctx, msg) },
		Device:     assetID,
		ExactConn:  agentConn,
		Accept:     AcceptReplyTo(agentmgr.MsgPackageList),
		Timeout:    timeout,
		Disconnect: agentConn.Done(),
	})
	if err != nil {
		switch agentmgr.RequestKind(err) {
		case agentmgr.RequestDuplicate, agentmgr.RequestBusy:
			return failed("package update preview request id is already in use; no changes applied")
		case agentmgr.RequestInvalid:
			return failed("failed to encode package update preview request; no changes applied")
		case agentmgr.RequestSendFailed:
			return failed("failed to send package update preview request to agent; no changes applied")
		case agentmgr.RequestCanceled:
			return failed("package update preview canceled; no changes applied: " + ctx.Err().Error())
		case agentmgr.RequestDisconnected:
			return failed("agent disconnected during package update preview; no changes applied")
		default:
			return failed("agent did not respond to package update preview in time; no changes applied")
		}
	}
	if err := ctx.Err(); err != nil {
		return failed("package update preview canceled; no changes applied: " + err.Error())
	}
	var listed packageListedResponseWire
	if err := json.Unmarshal(msg.Data, &listed); err != nil {
		return failed("invalid package update preview response; no changes applied")
	}
	if err := validatePackageListedResponse(listed, wireID, packageInventoryUpgradable); err != nil {
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
		d.writeAgentNotConnected(w, assetID)
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
	msg, err := d.requestAgent(d.PackageReplies, agentConn, agentmgr.MsgPackageAction, requestID,
		agentmgr.PackageActionData{RequestID: requestID, Action: action, Packages: cleanedPackages}, assetID, packageActionTimeout)
	if err != nil {
		d.writeAgentRequestFailure(w, assetID, err)
		return
	}
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
		servicehttp.WriteError(w, http.StatusBadRequest, agentFailureMessage(result.Error, result.Output))
		return
	}
	servicehttp.WriteJSON(w, http.StatusOK, result)
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
	deliverAgentReply(d.PackageReplies, matchPackage, agentmgr.MsgPackageListed, conn, msg, data.RequestID)
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
	deliverAgentReply(d.PackageReplies, matchPackage, agentmgr.MsgPackageResult, conn, msg, data.RequestID)
}
