package resources

import (
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/servicehttp"
)

const RestartSettingsRoute = "/settings/restart"

var HubRestartSelf = func() error {
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}

type RestartSettingsResponse struct {
	Accepted bool   `json:"accepted"`
	Message  string `json:"message"`
}

func (d *Deps) HandleRestartSettings(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != RestartSettingsRoute {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	auditEvent := audit.NewEvent("settings.restart.requested")
	auditEvent.ActorID = d.PrincipalActorID(r.Context())
	auditEvent.Decision = "applied"
	d.AppendAuditEventBestEffort(auditEvent, "api warning: failed to append restart request audit event")

	servicehttp.WriteJSON(w, http.StatusAccepted, RestartSettingsResponse{
		Accepted: true,
		Message:  "Backend restart requested. The hub will restart automatically only if it is managed by Docker or another process supervisor.",
	})

	go func() {
		time.Sleep(300 * time.Millisecond)
		if err := HubRestartSelf(); err != nil {
			// Best effort only.
		}
	}()
}
