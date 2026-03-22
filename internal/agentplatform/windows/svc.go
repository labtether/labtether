//go:build windows

package windows

import (
	"context"
	"log"
	"sync"
	"time"

	"golang.org/x/sys/windows/svc"

	"github.com/labtether/labtether/internal/agentcore"
)

const serviceName = "LabTetherAgent"

// agentService implements svc.Handler for the Windows Service Control Manager.
type agentService struct {
	cfg      agentcore.RuntimeConfig
	provider agentcore.TelemetryProvider
}

// Execute is the main service entry point called by the SCM.
func (s *agentService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (svcSpecificExitCode bool, exitCode uint32) {
	changes <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var runErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		runErr = agentcore.Run(ctx, s.cfg, s.provider)
	}()

	changes <- svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptShutdown,
	}

	// Wait for a stop/shutdown signal from the SCM.
	for {
		cr := <-r
		switch cr.Cmd {
		case svc.Interrogate:
			changes <- cr.CurrentStatus
		case svc.Stop, svc.Shutdown:
			log.Printf("labtether-agent: SCM requested %v, shutting down", cr.Cmd)
			changes <- svc.Status{State: svc.StopPending}
			cancel()

			// Wait for the agent goroutine to exit with a hard deadline.
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(30 * time.Second):
				log.Printf("labtether-agent: shutdown timed out after 30s")
			}

			if runErr != nil {
				log.Printf("labtether-agent: agent exited with error: %v", runErr)
				return false, 1
			}
			return false, 0
		default:
			log.Printf("labtether-agent: unexpected SCM control request: %d", cr.Cmd)
		}
	}
}

// IsWindowsService reports whether the process is running as a Windows Service.
func IsWindowsService() bool {
	isSvc, err := svc.IsWindowsService()
	if err != nil {
		log.Printf("labtether-agent: failed to detect Windows Service context: %v", err)
		return false
	}
	return isSvc
}

// RunAsService runs the agent under the Windows Service Control Manager.
func RunAsService(cfg agentcore.RuntimeConfig, provider agentcore.TelemetryProvider) error {
	return svc.Run(serviceName, &agentService{
		cfg:      cfg,
		provider: provider,
	})
}
