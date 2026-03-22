//go:build windows

package main

import (
	"fmt"
	"os"

	"github.com/labtether/labtether/internal/agentcore"
	"github.com/labtether/labtether/internal/agentplatform/windows"
)

func handleWindowsServiceArgs(args []string) (handled bool) {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "install":
		if err := windows.InstallService(); err != nil {
			fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
			os.Exit(1)
		}
		return true
	case "uninstall":
		if err := windows.UninstallService(); err != nil {
			fmt.Fprintf(os.Stderr, "uninstall failed: %v\n", err)
			os.Exit(1)
		}
		return true
	}
	return false
}

func isWindowsService() bool {
	return windows.IsWindowsService()
}

func runAsWindowsService(cfg agentcore.RuntimeConfig, provider agentcore.TelemetryProvider) error {
	return windows.RunAsService(cfg, provider)
}
