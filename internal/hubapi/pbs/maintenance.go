package pbs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/maintenanceguard"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) enforceMutation(w http.ResponseWriter, runtime *PBSRuntime, stores ...string) bool {
	encoded, err := json.Marshal(stores)
	if err != nil || runtime == nil {
		servicehttp.WriteError(w, http.StatusConflict, "cannot verify PBS mutation owner")
		return false
	}
	return maintenanceguard.EnforceConnectorAction(w, "pbs", "dedicated.mutate", connectorsdk.ActionRequest{
		Params: map[string]string{"collector_id": runtime.CollectorID, "_pbs_stores": string(encoded)},
	}, d.EvaluateConnectorGuardrails)
}

type pbsJobStores struct {
	store, remote, remoteStore string
}

func (j pbsJobStores) stores(sync bool) ([]string, error) {
	if j.store == "" {
		return nil, fmt.Errorf("PBS job datastore is missing")
	}
	out := []string{j.store}
	// PBS treats an omitted remote as local sync. Both local stores then
	// participate; a remote appliance's store name is never a local alias.
	if sync && j.remote == "" {
		if j.remoteStore == "" {
			return nil, fmt.Errorf("PBS local sync source datastore is missing")
		}
		out = append(out, j.remoteStore)
	}
	return out, nil
}

func (d *Deps) enforceJobMutation(ctx context.Context, w http.ResponseWriter, runtime *PBSRuntime, kind, id string, values url.Values) bool {
	if d.EvaluateConnectorGuardrails == nil {
		return true
	}
	current := pbsJobStores{}
	var stores []string
	if id != "" {
		var err error
		current, err = readPBSJobStores(ctx, runtime, kind, id)
		if err != nil {
			servicehttp.WriteError(w, http.StatusConflict, "cannot verify PBS job datastore")
			return false
		}
		stores, err = current.stores(kind == "sync")
		if err != nil {
			servicehttp.WriteError(w, http.StatusConflict, "cannot verify PBS job datastore")
			return false
		}
	}
	if values != nil {
		fields := map[string]*string{"store": &current.store, "remote": &current.remote, "remote-store": &current.remoteStore}
		for name, target := range fields {
			if raw, present := values[name]; present {
				if len(raw) != 1 {
					servicehttp.WriteError(w, http.StatusBadRequest, "ambiguous PBS job datastore fields")
					return false
				}
				*target = strings.TrimSpace(raw[0])
			}
		}
		for _, deleted := range values["delete"] {
			for _, name := range strings.Split(deleted, ",") {
				if target := fields[strings.TrimSpace(name)]; target != nil {
					*target = ""
				}
			}
		}
		updated, err := current.stores(kind == "sync")
		if err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "PBS job datastore is required")
			return false
		}
		stores = append(stores, updated...)
	}
	if err := ctx.Err(); err != nil {
		servicehttp.WriteError(w, http.StatusRequestTimeout, "PBS request canceled")
		return false
	}
	return d.enforceMutation(w, runtime, stores...)
}

func readPBSJobStores(ctx context.Context, runtime *PBSRuntime, kind, id string) (pbsJobStores, error) {
	var found []pbsJobStores
	switch kind {
	case "verify":
		jobs, err := runtime.Client.ListVerifyJobs(ctx)
		if err != nil {
			return pbsJobStores{}, err
		}
		for _, job := range jobs {
			if job.ID == id {
				found = append(found, pbsJobStores{store: job.Store})
			}
		}
	case "prune":
		jobs, err := runtime.Client.ListPruneJobs(ctx)
		if err != nil {
			return pbsJobStores{}, err
		}
		for _, job := range jobs {
			if job.ID == id {
				found = append(found, pbsJobStores{store: job.Store})
			}
		}
	case "sync":
		jobs, err := runtime.Client.ListSyncJobs(ctx)
		if err != nil {
			return pbsJobStores{}, err
		}
		for _, job := range jobs {
			if job.ID == id {
				found = append(found, pbsJobStores{job.Store, job.Remote, job.RemoteStore})
			}
		}
	default:
		return pbsJobStores{}, fmt.Errorf("unknown PBS job kind")
	}
	if len(found) != 1 {
		return pbsJobStores{}, fmt.Errorf("PBS job missing or ambiguous")
	}
	return found[0], nil
}
