package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/labtether/labtether/internal/agentcore"
	"github.com/labtether/labtether/internal/agentplatform"
)

func main() {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("LABTETHER_ALLOW_INSECURE_TRANSPORT")), "true") {
		log.Printf("labtether-agent: WARNING: insecure transport mode is enabled (LABTETHER_ALLOW_INSECURE_TRANSPORT=true)")
	}

	// Handle Windows-specific service install/uninstall commands before
	// any agent runtime setup (these exit on their own).
	if handleWindowsServiceArgs(os.Args[1:]) {
		return
	}

	cfg := agentcore.LoadConfig("labtether-agent", "8090", agentplatform.DefaultSource())
	if handled, exitCode := agentcore.HandleCLICommand(cfg, os.Args[1:]); handled {
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return
	}
	provider := agentplatform.NewProvider(cfg.AssetID, cfg.Source)

	// Determine whether to run as a Windows Service or in interactive mode.
	// The --console flag forces interactive mode even when launched by the SCM.
	forceConsole := hasFlag(os.Args[1:], "--console")
	if !forceConsole && isWindowsService() {
		log.Printf("labtether-agent: starting as Windows Service")
		if err := runAsWindowsService(cfg, provider); err != nil {
			log.Fatalf("%s service exited with error: %v", cfg.Name, err)
		}
		return
	}

	// Interactive (foreground) mode — signal-based lifecycle.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := agentcore.Run(ctx, cfg, provider); err != nil {
		log.Fatalf("%s exited with error: %v", cfg.Name, err)
	}
}

// hasFlag checks whether a flag appears in the argument list.
func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if strings.TrimSpace(arg) == flag {
			return true
		}
	}
	return false
}
