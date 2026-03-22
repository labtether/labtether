package sysconfig

import (
	"errors"
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
)

var ResolveNetworkMethodFn = ResolveNetworkMethod
var BackupNetplanConfigFn = BackupNetplanConfig
var RestoreNetplanConfigFn = RestoreNetplanConfig
var VerifyNetworkConnectivity = VerifyConnectivity
var CollectActiveNMConnectionsFn = CollectActiveNMConnections
var ActivateNMConnectionsFn = ActivateNMConnections

func (nm *NetworkManager) ApplyActionLinux(req agentmgr.NetworkActionData) agentmgr.NetworkResultData {
	result := agentmgr.NetworkResultData{RequestID: req.RequestID}

	method, err := ResolveNetworkMethodFn(req.Method)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	switch method {
	case "netplan":
		backupRef, backupErr := BackupNetplanConfigFn()
		if backupErr != nil {
			result.Error = fmt.Sprintf("failed to create netplan backup: %v", backupErr)
			return result
		}
		result.RollbackReference = backupRef

		nm.mu.Lock()
		nm.LastMethod = "netplan"
		nm.LastNetplanBackup = backupRef
		nm.LastNMConnections = nil
		nm.mu.Unlock()

		out, runErr := NetworkRunCommandWithTimeout(NetworkActionCommandTimeout, "netplan", "apply")
		result.Output = TruncateCommandOutput(out, MaxCommandOutputBytes)
		if runErr != nil {
			result.Error = fmt.Sprintf("netplan apply failed: %v", runErr)
			return result
		}

		if verifyErr := VerifyNetworkConnectivity(req.VerifyTarget); verifyErr != nil {
			rollbackOutput, rollbackErr := nm.rollbackNetplan()
			result.RollbackAttempted = true
			result.RollbackOutput = rollbackOutput
			result.RollbackSucceeded = rollbackErr == nil
			if rollbackErr != nil {
				result.Error = fmt.Sprintf("connectivity verification failed (%v); rollback failed: %v", verifyErr, rollbackErr)
				return result
			}
			result.Error = fmt.Sprintf("connectivity verification failed (%v); rollback applied", verifyErr)
			return result
		}

		result.OK = true
		return result
	case "nmcli":
		active, activeErr := CollectActiveNMConnectionsFn()
		if activeErr != nil {
			result.Error = fmt.Sprintf("failed to snapshot active network connections: %v", activeErr)
			return result
		}

		nm.mu.Lock()
		nm.LastMethod = "nmcli"
		nm.LastNMConnections = CloneStringSlice(active)
		nm.LastNetplanBackup = ""
		nm.mu.Unlock()

		nmcliArgs := []string{"connection", "reload"}
		if connection := strings.TrimSpace(req.Connection); connection != "" {
			nmcliArgs = []string{"connection", "up", connection}
		}
		out, runErr := NetworkRunCommandWithTimeout(NetworkActionCommandTimeout, "nmcli", nmcliArgs...)
		result.Output = TruncateCommandOutput(out, MaxCommandOutputBytes)
		if runErr != nil {
			result.Error = fmt.Sprintf("nmcli %s failed: %v", strings.Join(nmcliArgs, " "), runErr)
			return result
		}

		if verifyErr := VerifyNetworkConnectivity(req.VerifyTarget); verifyErr != nil {
			rollbackOutput, rollbackErr := nm.rollbackNMCLI()
			result.RollbackAttempted = true
			result.RollbackOutput = rollbackOutput
			result.RollbackSucceeded = rollbackErr == nil
			if rollbackErr != nil {
				result.Error = fmt.Sprintf("connectivity verification failed (%v); rollback failed: %v", verifyErr, rollbackErr)
				return result
			}
			result.Error = fmt.Sprintf("connectivity verification failed (%v); rollback applied", verifyErr)
			return result
		}

		result.OK = true
		return result
	default:
		result.Error = fmt.Sprintf("unsupported method %q", method)
		return result
	}
}

func (nm *NetworkManager) RollbackActionLinux(req agentmgr.NetworkActionData) agentmgr.NetworkResultData {
	result := agentmgr.NetworkResultData{RequestID: req.RequestID}

	method := strings.ToLower(strings.TrimSpace(req.Method))
	if method == "" || method == "auto" {
		nm.mu.Lock()
		method = nm.LastMethod
		nm.mu.Unlock()
	}
	if method == "" {
		result.Error = "no rollback snapshot is available yet"
		return result
	}

	result.RollbackAttempted = true

	switch method {
	case "netplan":
		output, err := nm.rollbackNetplan()
		result.RollbackOutput = output
		result.RollbackSucceeded = err == nil
		result.OK = err == nil
		if err != nil {
			result.Error = err.Error()
		}
		nm.mu.Lock()
		result.RollbackReference = nm.LastNetplanBackup
		nm.mu.Unlock()
		return result
	case "nmcli":
		output, err := nm.rollbackNMCLI()
		result.RollbackOutput = output
		result.RollbackSucceeded = err == nil
		result.OK = err == nil
		if err != nil {
			result.Error = err.Error()
		}
		return result
	default:
		result.Error = fmt.Sprintf("invalid rollback method %q", method)
		return result
	}
}

func (nm *NetworkManager) rollbackNetplan() (string, error) {
	nm.mu.Lock()
	backupRef := strings.TrimSpace(nm.LastNetplanBackup)
	nm.mu.Unlock()

	if backupRef == "" {
		return "", errors.New("no netplan backup reference is available")
	}
	if err := RestoreNetplanConfigFn(backupRef); err != nil {
		return "", fmt.Errorf("restore netplan backup: %w", err)
	}
	out, err := NetworkRunCommandWithTimeout(NetworkActionCommandTimeout, "netplan", "apply")
	output := TruncateCommandOutput(out, MaxCommandOutputBytes)
	if err != nil {
		return output, fmt.Errorf("netplan apply after restore failed: %w", err)
	}
	return output, nil
}

func (nm *NetworkManager) rollbackNMCLI() (string, error) {
	nm.mu.Lock()
	connections := CloneStringSlice(nm.LastNMConnections)
	nm.mu.Unlock()

	if len(connections) == 0 {
		return "", errors.New("no active nmcli connection snapshot is available")
	}
	return ActivateNMConnectionsFn(connections)
}
