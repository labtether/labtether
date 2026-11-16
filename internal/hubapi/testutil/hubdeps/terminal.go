package hubdeps

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/hubapi/dockerpkg"
	"github.com/labtether/labtether/internal/hubapi/operations"
	"github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/hubapi/shared"
	terminalpkg "github.com/labtether/labtether/internal/hubapi/terminal"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/truenas"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/terminal"
)

// cmd/labtether's validation constants of the same names (main.go).
const (
	maxActorIDLength = 64
	maxCommandLength = 4096
	maxModeLength    = 32
)

// BuildTerminalDeps mirrors cmd's buildTerminalDeps field by field. Stand-ins
// replace only cmd's middleware (route auth, rate limit, stream tickets and
// the WebSocket origin check); see testutil/standins.go for what each one
// still refuses. Like cmd's test server, the Hub has no database and no
// Docker coordinator, so no target resolves to a container.
func (h *Hub) BuildTerminalDeps() *terminalpkg.Deps {
	if h.EphemeralSSHConfigs == nil {
		h.EphemeralSSHConfigs = terminalpkg.NewEphemeralSSHConfigStore(0)
	}
	return &terminalpkg.Deps{
		SessionStreams:          &h.SessionStreams,
		TerminalStore:           h.TerminalStore,
		TerminalPersistentStore: h.TerminalPersistentStore,
		TerminalBookmarkStore:   h.TerminalBookmarkStore,
		SSHKnownHostStore:       h.SSHKnownHostStore,
		TerminalScrollbackStore: h.TerminalScrollbackStore,
		CredentialStore:         h.CredentialStore,
		AuditStore:              h.AuditStore,
		LogStore:                h.LogStore,
		AssetStore:              h.AssetStore,
		GroupStore:              h.GroupStore,

		DBPool: nil,

		AgentMgr: h.AgentMgr,
		JobQueue: h.JobQueue,

		PolicyState:    h.PolicyState,
		SecretsManager: h.SecretsManager,

		TerminalBridges:  &h.TerminalBridges,
		PendingAgentCmds: &h.PendingAgentCmds,

		TerminalWebSocketUpgrader: *testutil.NewStreamUpgrader(),

		EnforceRateLimit: h.RateLimiter.Enforce,
		WrapAuth:         testutil.RouteAuthNotTested(h.t),

		IssueStreamTicket:       h.StreamTickets.Issue,
		ConsumeStreamTicketAuth: testutil.StreamTicketAuthNotTested(h.t),
		PrincipalActorID:        apiv2.PrincipalActorID,
		DecodeJSONBody:          shared.DecodeJSONBody,
		ValidateMaxLen:          shared.ValidateMaxLen,
		AppendAuditEventBestEffort: func(event audit.Event, logMessage string) {
			shared.AppendAuditEventBestEffort(h.AuditStore, event, logMessage)
		},
		AppendLogEventBestEffort: func(event logs.Event, logMessage string) {
			shared.AppendLogEventBestEffort(h.LogStore, event, logMessage)
		},
		EvaluateAssetGuardrails: h.GroupFeaturesDeps().EvaluateAssetGuardrails,

		ResolveDockerExecSessionTarget: (&dockerpkg.Deps{}).ResolveDockerExecSessionTarget,
		HandleDockerExecTerminalStream: dockerExecNotWired(h.t),
		ResolveProxmoxSessionTarget: func(assetID string) (terminalpkg.ProxmoxSessionTarget, bool, error) {
			target, ok, err := h.ProxmoxDeps().ResolveProxmoxSessionTarget(assetID)
			return target, ok, err
		},
		TryProxmoxTerminalStream: func(w http.ResponseWriter, r *http.Request, session terminal.Session, target terminalpkg.ProxmoxSessionTarget) error {
			return h.ProxmoxDeps().TryProxmoxTerminalStream(w, r, session, target.(proxmox.ProxmoxSessionTarget))
		},
		ResolveTrueNASSessionTarget: func(assetID string) (terminalpkg.TruenasShellTarget, bool, error) {
			target, ok, err := h.TrueNASDeps().ResolveTrueNASSessionTarget(assetID)
			return target, ok, err
		},
		TryTrueNASTerminalStream: func(w http.ResponseWriter, r *http.Request, session terminal.Session, target terminalpkg.TruenasShellTarget) error {
			return h.TrueNASDeps().TryTrueNASTerminalStream(w, r, session, target.(truenas.TruenasShellTarget))
		},

		GetProtocolConfig: func(context.Context, string, string) (*protocols.ProtocolConfig, error) {
			return nil, nil // cmd's closure answers this when it has no database
		},

		GetAssetTerminalConfig: func(assetID string) (credentials.AssetTerminalConfig, bool, error) {
			if h.CredentialStore == nil {
				return credentials.AssetTerminalConfig{}, false, errors.New("credential store not configured")
			}
			return h.CredentialStore.GetAssetTerminalConfig(assetID)
		},
		SaveAssetTerminalConfig: func(cfg credentials.AssetTerminalConfig) (credentials.AssetTerminalConfig, error) {
			if h.CredentialStore == nil {
				return credentials.AssetTerminalConfig{}, errors.New("credential store not configured")
			}
			return h.CredentialStore.SaveAssetTerminalConfig(cfg)
		},
		HubSSHKeyEnabled: hubSSHKeyNotTested(h.t),

		TerminalInMemStore:  h.TerminalInMemStore,
		EphemeralSSHConfigs: h.EphemeralSSHConfigs,

		ExecuteSSHCommandFn: func(job terminal.CommandJob, mode string, timeout time.Duration, maxOutput int) (string, error) {
			return operations.ExecuteSSHCommand(job, operations.CommandExecutorConfig{
				Mode:           mode,
				Timeout:        timeout,
				MaxOutputBytes: maxOutput,
			})
		},

		MaxActorIDLength: maxActorIDLength,
		MaxTargetLength:  maxTargetLength,
		MaxCommandLength: maxCommandLength,
		MaxModeLength:    maxModeLength,
	}
}

// TerminalDeps mirrors cmd's ensureTerminalDeps: built on first use and
// kept, so the stream owners it holds last for the test. A test that swaps
// a store or the agent manager after first use calls ResetTerminalDeps, as
// cmd tests call resetTerminalDepsForTest.
func (h *Hub) TerminalDeps() *terminalpkg.Deps {
	h.terminalOnce.Do(func() { h.terminalDeps = h.BuildTerminalDeps() })
	return h.terminalDeps
}

// ResetTerminalDeps mirrors cmd's resetTerminalDepsForTest.
func (h *Hub) ResetTerminalDeps() {
	h.terminalDeps = nil
	h.terminalOnce = sync.Once{}
}

// hubSSHKeyNotTested stands in for cmd's hubSSHKeyEnabled, which decides
// whether the Hub's own SSH key may be used for a device. It fails the test
// if a request reaches it and answers "off", the safe side; that rule is
// tested in cmd.
func hubSSHKeyNotTested(t *testing.T) func(string) bool {
	return func(string) bool {
		t.Errorf("a request reached the Hub SSH key stand-in; that check is tested in cmd/labtether")
		return false
	}
}

// dockerExecNotWired stands in for the Docker package's exec stream, which
// has no builder here yet. No target resolves to a container without a
// Docker coordinator, so a request that reaches it fails the test.
func dockerExecNotWired(t *testing.T) func(http.ResponseWriter, *http.Request, terminal.Session, string, string) {
	return func(w http.ResponseWriter, _ *http.Request, _ terminal.Session, _, _ string) {
		t.Errorf("a stream reached the Docker exec stand-in; Docker is not wired in hubdeps yet")
		servicehttp.WriteError(w, http.StatusInternalServerError, "docker exec is not wired in package tests")
	}
}
