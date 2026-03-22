package backends

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
)

// PackageManager handles package inventory requests from the hub.
type PackageManager struct {
	Backend PackageBackend
}

// NewPackageManager creates a PackageManager with the OS-appropriate backend.
func NewPackageManager() *PackageManager {
	return &PackageManager{
		Backend: NewPackageBackendForOS(),
	}
}

// CloseAll is a no-op for PackageManager — package requests are stateless
// and require no cleanup.
func (pm *PackageManager) CloseAll() {}

// HandlePackageList collects installed packages and sends them to the hub.
func (pm *PackageManager) HandlePackageList(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.PackageListData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("package: invalid package.list request: %v", err)
		return
	}

	pkgs, err := pm.Backend.ListPackages()

	var errMsg string
	if err != nil {
		errMsg = err.Error()
		log.Printf("package: failed to collect packages: %v", err)
	}

	data, marshalErr := json.Marshal(agentmgr.PackageListedData{
		RequestID: req.RequestID,
		Packages:  pkgs,
		Error:     errMsg,
	})
	if marshalErr != nil {
		log.Printf("package: failed to marshal package.listed response: %v", marshalErr)
		return
	}

	if sendErr := transport.Send(agentmgr.Message{
		Type: agentmgr.MsgPackageListed,
		ID:   req.RequestID,
		Data: data,
	}); sendErr != nil {
		log.Printf("package: failed to send package.listed for request %s: %v", req.RequestID, sendErr)
	}
}

// HandlePackageAction performs a package-manager operation and returns result details.
func (pm *PackageManager) HandlePackageAction(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.PackageActionData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("package: invalid package.action request: %v", err)
		return
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action != "install" && action != "remove" && action != "upgrade" {
		pm.sendPackageResult(transport, req.RequestID, false, "", "invalid package action", false)
		return
	}

	packages := NormalizePackageList(req.Packages)
	if (action == "install" || action == "remove") && len(packages) == 0 {
		pm.sendPackageResult(transport, req.RequestID, false, "", "at least one package is required", false)
		return
	}

	result, err := pm.Backend.PerformAction(action, packages)
	if err != nil {
		pm.sendPackageResult(transport, req.RequestID, false, result.Output, err.Error(), result.RebootRequired)
		return
	}

	pm.sendPackageResult(transport, req.RequestID, true, result.Output, "", result.RebootRequired)
}

func (pm *PackageManager) sendPackageResult(
	transport MessageSender,
	requestID string,
	ok bool,
	output,
	errMsg string,
	rebootRequired bool,
) {
	data, marshalErr := json.Marshal(agentmgr.PackageResultData{
		RequestID:      requestID,
		OK:             ok,
		Output:         output,
		Error:          errMsg,
		RebootRequired: rebootRequired,
	})
	if marshalErr != nil {
		log.Printf("package: failed to marshal package.result: %v", marshalErr)
		return
	}

	if sendErr := transport.Send(agentmgr.Message{
		Type: agentmgr.MsgPackageResult,
		ID:   requestID,
		Data: data,
	}); sendErr != nil {
		log.Printf("package: failed to send package.result for request %s: %v", requestID, sendErr)
	}
}

// NormalizePackageList deduplicates and trims package names.
func NormalizePackageList(packages []string) []string {
	seen := make(map[string]struct{}, len(packages))
	result := make([]string, 0, len(packages))
	for _, pkg := range packages {
		name := strings.TrimSpace(pkg)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	return result
}
