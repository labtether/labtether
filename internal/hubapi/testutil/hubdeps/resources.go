package hubdeps

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/hubapi/collectors"
	"github.com/labtether/labtether/internal/hubapi/resources"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

// BuildResourcesDeps mirrors cmd's buildResourcesDeps field by field. Like
// cmd's test server, the Hub has no database (the database-backed stores are
// its nil Postgres store, and there is no manual device database), no data
// directory (transfers stage in the system temp directory), and no event
// broadcaster, so events are dropped as cmd drops them when it has none.
//
// Stand-ins replace cmd's route auth and rate limit (see
// testutil/standins.go) and the fields cmd fills with its own code: the Hub
// SSH key pieces and agent command dispatch. The asset sub-handlers that cmd
// takes from the auth and admin packages have no builder here yet. Each of
// these fails the test if reached. The canonical heartbeat writer is the
// resources package function cmd's persistCanonicalHeartbeat calls, over
// the Hub's canonical store. The desktop display list and the saved
// terminal session cleanup are the real desktop and terminal Deps.
func (h *Hub) BuildResourcesDeps() *resources.Deps {
	var db *persistence.PostgresStore
	enrollmentTransactions, _ := h.EnrollmentStore.(persistence.AgentEnrollmentTransactionStore)
	fileTransferStore := h.FileTransferStore
	if fileTransferStore == nil {
		fileTransferStore = db
	}
	return &resources.Deps{
		AgentMgr:            h.AgentMgr,
		AssetStore:          h.AssetStore,
		GroupStore:          h.GroupStore,
		GroupProfileStore:   h.GroupProfileStore,
		FailoverStore:       h.FailoverStore,
		TelemetryStore:      h.TelemetryStore,
		LogStore:            h.LogStore,
		DependencyStore:     h.DependencyStore,
		EdgeStore:           h.EdgeStore,
		SyntheticStore:      h.SyntheticStore,
		LinkSuggestionStore: h.LinkSuggestionStore,
		CredentialStore:     h.CredentialStore,
		AuditStore:          h.AuditStore,
		RetentionStore:      h.RetentionStore,
		DB:                  db,
		FileProtoPool:       h.FileProtoPool,
		FileConnectionStore: db,
		FileTransferStore:   fileTransferStore,
		ActiveTransfers:     &h.ActiveTransfers,
		SpoolDir:            "", // cmd's fileSpoolDir with no data directory
		RemoteBookmarkStore: db,

		FileBridges:    &h.FileBridges,
		ProcessReplies: &h.ProcessReplies,
		ServiceReplies: &h.ServiceReplies,
		JournalReplies: &h.JournalReplies,
		DiskReplies:    &h.DiskReplies,
		NetworkReplies: &h.NetworkReplies,
		WoLPending:     &h.WoLPending,
		PackageReplies: &h.PackageReplies,
		CronReplies:    &h.CronReplies,
		UsersReplies:   &h.UsersReplies,

		WrapAuth: testutil.RouteAuthNotTested(h.t),

		DecodeJSONBody:  shared.DecodeJSONBody,
		ExecuteViaAgent: agentCommandNotTested(h.t),

		EnforceRateLimit:        h.RateLimiter.Enforce,
		EvaluateAssetGuardrails: h.GroupFeaturesDeps().EvaluateAssetGuardrails,
		PrincipalActorID:        apiv2.PrincipalActorID,
		UserIDFromContext:       apiv2.UserIDFromContext,
		SecretsManager:          h.SecretsManager,
		AppendAuditEventBestEffort: func(event audit.Event, logMessage string) {
			shared.AppendAuditEventBestEffort(h.AuditStore, event, logMessage)
		},
		Broadcast: func(string, map[string]any) {},
		// cmd's ensureDefaultRelayPushRoute returns at once with no database.
		OnRelayPushDeviceRegistered: func(context.Context) {},

		ManualDeviceDB: nil, // cmd passes nil with no database

		EnrollmentStore:        h.EnrollmentStore,
		EnrollmentTransactions: enrollmentTransactions,
		HubCollectorStore:      h.HubCollectorStore,
		RemoveDockerHost: func(assetID string) {
			if h.DockerCoordinator != nil {
				h.DockerCoordinator.RemoveHost(assetID)
			}
		},
		RemoveWebServiceHost: func(assetID string) {
			if h.WebServiceCoordinator != nil {
				h.WebServiceCoordinator.RemoveHost(assetID)
			}
		},
		RemoveSavedTerminalSessions: func(ctx context.Context, assetID string) {
			if err := h.TerminalDeps().RemovePersistentSessionsForRemovedDevice(context.WithoutCancel(ctx), assetID); err != nil {
				h.t.Logf("failed to remove saved terminal sessions of removed device %s: %v", assetID, err)
			}
		},
		SendSSHKeyRemoveToAsset: hubSSHKeyRemovalNotTested(h.t),
		// cmd's persistCanonicalHeartbeat passes apiServer's canonical store,
		// read at the call, and the current time to this package function.
		PersistCanonicalHeartbeatFn: func(assetEntry assets.Asset, req assets.HeartbeatRequest) {
			resources.PersistCanonicalHeartbeatForAsset(h.CanonicalStore, assetEntry, req, time.Now().UTC())
		},
		CollectorConfigString:      collectors.CollectorConfigString,
		NormalizeAssetKey:          collectors.NormalizeAssetKey,
		AutoDockerCollectorAssetID: collectors.AutoDockerCollectorAssetID,

		HandleDesktopCredentials:         assetSubHandlerNotWired(h.t, "desktop credentials"),
		HandleRetrieveDesktopCredentials: assetSubHandlerNotWired(h.t, "desktop credential retrieval"),
		HandleDisplayList: func(w http.ResponseWriter, r *http.Request, assetID string) {
			h.DesktopDeps().HandleDisplayListDirect(w, r, assetID)
		},
		HandlePushHubKey: func(w http.ResponseWriter, r *http.Request, _ string) {
			assetSubHandlerNotWired(h.t, "Hub SSH key push")(w, r)
		},
		HandleTestProtocolConnection: func(w http.ResponseWriter, r *http.Request, _, _ string) {
			assetSubHandlerNotWired(h.t, "protocol connection test")(w, r)
		},
		HandleListProtocolConfigs: func(w http.ResponseWriter, r *http.Request, _ string) {
			assetSubHandlerNotWired(h.t, "protocol config list")(w, r)
		},
		HandleCreateProtocolConfig: func(w http.ResponseWriter, r *http.Request, _ string) {
			assetSubHandlerNotWired(h.t, "protocol config create")(w, r)
		},
		HandleUpdateProtocolConfig: func(w http.ResponseWriter, r *http.Request, _, _ string) {
			assetSubHandlerNotWired(h.t, "protocol config update")(w, r)
		},
		HandleDeleteProtocolConfig: func(w http.ResponseWriter, r *http.Request, _, _ string) {
			assetSubHandlerNotWired(h.t, "protocol config delete")(w, r)
		},
		HandleHubSSHKey: func(w http.ResponseWriter, r *http.Request, _ assets.Asset) {
			assetSubHandlerNotWired(h.t, "Hub SSH key setting")(w, r)
		},
		HubSSHKeyEnabled: hubSSHKeyNotTested(h.t),
	}
}

// ResourcesDeps mirrors cmd's ensureResourcesDeps: a new build per call, so
// a test's store changes are seen at once.
func (h *Hub) ResourcesDeps() *resources.Deps {
	return h.BuildResourcesDeps()
}

// hubSSHKeyRemovalNotTested stands in for the closure in cmd's
// buildResourcesDeps that tells a removed device's agent to drop the Hub's
// SSH key. It reads cmd's Hub SSH identity. It fails the test if reached.
func hubSSHKeyRemovalNotTested(t *testing.T) func(string) {
	return func(string) {
		t.Errorf("a device removal reached the Hub SSH key removal stand-in; that step is tested in cmd/labtether")
	}
}

// agentCommandNotTested stands in for cmd's executeViaAgent, which runs a
// command through the agents package's Deps. That package has no builder
// here yet. It fails the test if reached and returns a failed result.
func agentCommandNotTested(t *testing.T) func(terminal.CommandJob) terminal.CommandResult {
	return func(job terminal.CommandJob) terminal.CommandResult {
		t.Errorf("command %q reached agent command dispatch; it is not wired in package tests", job.CommandID)
		return terminal.CommandResult{JobID: job.JobID, SessionID: job.SessionID, CommandID: job.CommandID,
			Status: "failed", Output: "agent command dispatch is not wired in package tests", CompletedAt: time.Now().UTC()}
	}
}

// assetSubHandlerNotWired stands in for a device sub-handler that cmd takes
// from its own code or from a package with no builder here (auth, admin).
// It fails the test and answers 500 if a request reaches it.
func assetSubHandlerNotWired(t *testing.T, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		t.Errorf("a request reached the %s stand-in; it is not wired in package tests", name)
		servicehttp.WriteError(w, http.StatusInternalServerError, name+" is not wired in package tests")
	}
}
