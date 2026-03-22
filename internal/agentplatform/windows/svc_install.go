//go:build windows

package windows

import (
	"fmt"
	"log"
	"os"
	"time"

	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

// InstallService registers LabTetherAgent as a Windows Service with auto-start
// and failure recovery actions (restart 3 times with 60-second delays).
func InstallService() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to SCM: %w", err)
	}
	defer m.Disconnect()

	// Check if the service already exists.
	existing, err := m.OpenService(serviceName)
	if err == nil {
		existing.Close()
		return fmt.Errorf("service %q already exists; uninstall first", serviceName)
	}

	s, err := m.CreateService(serviceName, exePath, mgr.Config{
		DisplayName: "LabTether Agent",
		Description: "LabTether endpoint agent for telemetry, remote access, and automation.",
		StartType:   mgr.StartAutomatic,
	})
	if err != nil {
		return fmt.Errorf("creating service: %w", err)
	}
	defer s.Close()

	// Configure recovery actions: restart 3 times with 60-second delays.
	recoveryActions := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
	}
	if err := s.SetRecoveryActions(recoveryActions, 86400); err != nil {
		log.Printf("labtether-agent: warning: failed to set recovery actions: %v", err)
	}

	// Install the event log source so the agent can write to the Application log.
	if err := eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil {
		log.Printf("labtether-agent: warning: failed to install event log source: %v", err)
	}

	fmt.Printf("Service %q installed successfully.\n", serviceName)
	fmt.Println("Start the service with: sc start LabTetherAgent")
	return nil
}

// UninstallService removes the LabTetherAgent service registration and its
// event log source from the system.
func UninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("opening service %q: %w", serviceName, err)
	}
	defer s.Close()

	if err := s.Delete(); err != nil {
		return fmt.Errorf("deleting service: %w", err)
	}

	// Remove the event log source (best-effort).
	if err := eventlog.Remove(serviceName); err != nil {
		log.Printf("labtether-agent: warning: failed to remove event log source: %v", err)
	}

	fmt.Printf("Service %q uninstalled successfully.\n", serviceName)
	return nil
}
