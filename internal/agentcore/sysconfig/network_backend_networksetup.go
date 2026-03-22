package sysconfig

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

type DarwinNetworkBackend struct{}

type DarwinNetworkSnapshot struct {
	Service         string
	DNSServers      []string
	HasDNSServers   bool
	Enabled         bool
	HasEnabledState bool
}

func (DarwinNetworkBackend) ApplyAction(nm *NetworkManager, req agentmgr.NetworkActionData) agentmgr.NetworkResultData {
	return nm.applyActionDarwin(req)
}

func (DarwinNetworkBackend) RollbackAction(nm *NetworkManager, req agentmgr.NetworkActionData) agentmgr.NetworkResultData {
	return nm.rollbackActionDarwin(req)
}

func (nm *NetworkManager) applyActionDarwin(req agentmgr.NetworkActionData) agentmgr.NetworkResultData {
	result := agentmgr.NetworkResultData{RequestID: req.RequestID}

	method, err := ResolveDarwinNetworkMethod(req.Method)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if method != "networksetup" {
		result.Error = fmt.Sprintf("unsupported method %q", method)
		return result
	}

	service, err := ResolveDarwinNetworkService(req.Connection)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	snapshot, snapshotErr := CaptureDarwinNetworkSetupSnapshot(service)
	if snapshotErr != nil {
		result.Error = fmt.Sprintf("failed to snapshot network state: %v", snapshotErr)
		return result
	}
	result.RollbackReference = snapshot.Service

	nm.mu.Lock()
	nm.LastMethod = "networksetup"
	nm.LastDarwinSnapshot = CloneDarwinNetworkSnapshot(snapshot)
	nm.LastNetplanBackup = ""
	nm.LastNMConnections = nil
	nm.mu.Unlock()

	out, runErr := RunCommandWithTimeout(NetworkActionCommandTimeout, "networksetup", "-setnetworkserviceenabled", service, "on")
	result.Output = TruncateCommandOutput(out, MaxCommandOutputBytes)
	if runErr != nil {
		result.Error = fmt.Sprintf("networksetup apply failed: %v", runErr)
		return result
	}

	if verifyErr := VerifyDarwinConnectivity(req.VerifyTarget); verifyErr != nil {
		rollbackOutput, rollbackErr := nm.rollbackDarwinNetworkSetup()
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
}

func (nm *NetworkManager) rollbackActionDarwin(req agentmgr.NetworkActionData) agentmgr.NetworkResultData {
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
	if method != "networksetup" {
		result.Error = fmt.Sprintf("invalid rollback method %q", method)
		return result
	}

	result.RollbackAttempted = true
	output, err := nm.rollbackDarwinNetworkSetup()
	result.RollbackOutput = output
	result.RollbackSucceeded = err == nil
	result.OK = err == nil
	nm.mu.Lock()
	if nm.LastDarwinSnapshot != nil {
		result.RollbackReference = nm.LastDarwinSnapshot.Service
	}
	nm.mu.Unlock()
	if err != nil {
		result.Error = err.Error()
	}

	return result
}

func (nm *NetworkManager) rollbackDarwinNetworkSetup() (string, error) {
	nm.mu.Lock()
	snapshot := CloneDarwinNetworkSnapshot(nm.LastDarwinSnapshot)
	nm.mu.Unlock()

	if snapshot == nil {
		return "", errors.New("no networksetup snapshot is available")
	}

	parts := make([]string, 0, 2)
	var firstErr error

	if snapshot.HasEnabledState {
		state := "off"
		if snapshot.Enabled {
			state = "on"
		}
		out, err := RunCommandWithTimeout(NetworkActionCommandTimeout, "networksetup", "-setnetworkserviceenabled", snapshot.Service, state)
		trimmed := TruncateCommandOutput(out, MaxCommandOutputBytes)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("restore network service enabled state: %w", err)
		}
	}

	dnsArgs := []string{"-setdnsservers", snapshot.Service, "empty"}
	if snapshot.HasDNSServers && len(snapshot.DNSServers) > 0 {
		dnsArgs = append([]string{"-setdnsservers", snapshot.Service}, snapshot.DNSServers...)
	}
	out, err := RunCommandWithTimeout(NetworkActionCommandTimeout, "networksetup", dnsArgs...)
	trimmed := TruncateCommandOutput(out, MaxCommandOutputBytes)
	if trimmed != "" {
		parts = append(parts, trimmed)
	}
	if err != nil && firstErr == nil {
		firstErr = fmt.Errorf("restore DNS servers: %w", err)
	}

	return strings.Join(parts, "\n"), firstErr
}

func ResolveDarwinNetworkMethod(raw string) (string, error) {
	return ResolveDarwinNetworkMethodWith(raw, HasCommand)
}

func ResolveDarwinNetworkMethodWith(raw string, commandExists func(string) bool) (string, error) {
	method := strings.ToLower(strings.TrimSpace(raw))
	switch method {
	case "", "auto":
		if !commandExists("networksetup") {
			return "", errors.New("networksetup is not installed")
		}
		return "networksetup", nil
	case "networksetup":
		if !commandExists("networksetup") {
			return "", errors.New("networksetup is not installed")
		}
		return "networksetup", nil
	default:
		return "", fmt.Errorf("invalid method %q: must be auto or networksetup", method)
	}
}

type DarwinNetworkService struct {
	Name     string
	Disabled bool
}

func ResolveDarwinNetworkService(raw string) (string, error) {
	if service := strings.TrimSpace(raw); service != "" {
		return service, nil
	}

	services, err := ListDarwinNetworkServices()
	if err != nil {
		return "", err
	}
	for _, service := range services {
		if service.Name != "" && !service.Disabled {
			return service.Name, nil
		}
	}
	for _, service := range services {
		if service.Name != "" {
			return service.Name, nil
		}
	}

	return "", errors.New("no network service is available for apply")
}

func ListDarwinNetworkServices() ([]DarwinNetworkService, error) {
	out, err := RunCommandWithTimeout(10*time.Second, "networksetup", "-listallnetworkservices")
	if err != nil {
		trimmed := TruncateCommandOutput(out, MaxCommandOutputBytes)
		if trimmed == "" {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %s", err, trimmed)
	}
	services := ParseDarwinNetworkServicesOutput(string(out))
	if len(services) == 0 {
		return nil, errors.New("no network services were returned by networksetup")
	}
	return services, nil
}

func ParseDarwinNetworkServicesOutput(raw string) []DarwinNetworkService {
	lines := strings.Split(raw, "\n")
	services := make([]DarwinNetworkService, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "an asterisk") {
			continue
		}

		disabled := strings.HasPrefix(line, "*")
		name := strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if name == "" {
			continue
		}
		services = append(services, DarwinNetworkService{
			Name:     name,
			Disabled: disabled,
		})
	}
	return services
}

func CaptureDarwinNetworkSetupSnapshot(service string) (*DarwinNetworkSnapshot, error) {
	dnsOut, dnsErr := RunCommandWithTimeout(10*time.Second, "networksetup", "-getdnsservers", service)
	if dnsErr != nil {
		trimmed := TruncateCommandOutput(dnsOut, MaxCommandOutputBytes)
		if trimmed == "" {
			return nil, dnsErr
		}
		return nil, fmt.Errorf("%w: %s", dnsErr, trimmed)
	}
	dnsServers, hasDNS := ParseDarwinDNSServersOutput(string(dnsOut))

	enabledOut, enabledErr := RunCommandWithTimeout(10*time.Second, "networksetup", "-getnetworkserviceenabled", service)
	enabled := false
	hasEnabledState := false
	if enabledErr == nil {
		enabled, hasEnabledState = ParseDarwinNetworkServiceEnabledOutput(string(enabledOut))
	}

	return &DarwinNetworkSnapshot{
		Service:         service,
		DNSServers:      dnsServers,
		HasDNSServers:   hasDNS,
		Enabled:         enabled,
		HasEnabledState: hasEnabledState,
	}, nil
}

func ParseDarwinDNSServersOutput(raw string) ([]string, bool) {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	servers := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.Contains(strings.ToLower(trimmed), "there aren't any dns servers set on") {
			return nil, false
		}
		servers = append(servers, trimmed)
	}
	if len(servers) == 0 {
		return nil, false
	}
	return servers, true
}

func ParseDarwinNetworkServiceEnabledOutput(raw string) (bool, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "enabled":
		return true, true
	case "disabled":
		return false, true
	default:
		return false, false
	}
}

func VerifyDarwinConnectivity(rawTarget string) error {
	target := strings.TrimSpace(rawTarget)
	if target == "" {
		target = DefaultConnectivityProbeHost
	}

	if !HasCommand("ping") {
		return nil
	}
	pingOut, pingErr := RunCommandWithTimeout(NetworkConnectivityTimeout, "ping", "-c", "1", "-W", "2000", target)
	if pingErr != nil {
		trimmed := TruncateCommandOutput(pingOut, MaxCommandOutputBytes)
		if trimmed == "" {
			return fmt.Errorf("ping %s failed: %w", target, pingErr)
		}
		return fmt.Errorf("ping %s failed: %s", target, trimmed)
	}
	return nil
}

func CloneDarwinNetworkSnapshot(snapshot *DarwinNetworkSnapshot) *DarwinNetworkSnapshot {
	if snapshot == nil {
		return nil
	}
	clone := *snapshot
	clone.DNSServers = CloneStringSlice(snapshot.DNSServers)
	return &clone
}
