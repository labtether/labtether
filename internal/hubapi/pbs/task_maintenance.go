package pbs

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	pbsconnector "github.com/labtether/labtether/internal/connectors/pbs"
	"github.com/labtether/labtether/internal/servicehttp"
)

var pbsDatastoreName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

func (d *Deps) enforceTaskStop(ctx context.Context, w http.ResponseWriter, runtime *PBSRuntime, node, upid string) bool {
	if d.EvaluateConnectorGuardrails == nil {
		return true
	}
	status, err := runtime.Client.GetTaskStatus(ctx, node, upid)
	if err != nil || status.UPID != upid {
		servicehttp.WriteError(w, http.StatusConflict, "cannot verify PBS task owner")
		return false
	}
	stores, err := pbsTaskStores(status)
	if err != nil {
		servicehttp.WriteError(w, http.StatusConflict, "cannot verify PBS task datastore")
		return false
	}
	return d.enforceMutation(w, runtime, stores...)
}

// The selected server's task status, rather than an untrusted URL UPID or a
// substring match, supplies the worker identity. These formats follow PBS's
// node/tasks check_job_store and scheduled verification/sync workers.
func pbsTaskStores(task pbsconnector.TaskStatus) ([]string, error) {
	parts := strings.Split(task.ID, ":")
	var stores []string
	switch task.Type {
	case "garbage_collection", "verify", "verify_group", "verify_snapshot", "verificationjob", "prune", "prunejob", "backup":
		stores = []string{parts[0]}
	case "syncjob":
		if len(parts) < 4 {
			return nil, fmt.Errorf("invalid PBS sync worker identity")
		}
		stores = []string{parts[2]}
		if parts[0] == "-" {
			stores = append(stores, parts[1])
		}
	default:
		return nil, fmt.Errorf("PBS task datastore identity is not supported")
	}
	for _, store := range stores {
		if !pbsDatastoreName.MatchString(store) {
			return nil, fmt.Errorf("invalid PBS task datastore")
		}
	}
	return stores, nil
}
