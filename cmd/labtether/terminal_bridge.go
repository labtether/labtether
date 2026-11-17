package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/credentials"
	proxmoxpkg "github.com/labtether/labtether/internal/hubapi/proxmox"
	"github.com/labtether/labtether/internal/hubapi/shared"
	terminalpkg "github.com/labtether/labtether/internal/hubapi/terminal"
	truenaspkg "github.com/labtether/labtether/internal/hubapi/truenas"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/protocols"
	"github.com/labtether/labtether/internal/terminal"
)

func dbPoolOrNil(db *persistence.PostgresStore) *pgxpool.Pool {
	if db == nil {
		return nil
	}
	return db.Pool()
}

// buildTerminalDeps constructs the terminal.Deps from the apiServer's fields.
// Phase 1 populates all store/config fields. Phase 2 wires SSH helper function
// fields to the Deps methods that now own those implementations, using closures
// that capture d to avoid circular assignment during struct initialization.
func (s *apiServer) buildTerminalDeps() *terminalpkg.Deps {
	if s.ephemeralSSHConfigs == nil {
		s.ephemeralSSHConfigs = terminalpkg.NewEphemeralSSHConfigStore(0)
	}
	d := &terminalpkg.Deps{
		SessionStreams:          &s.sessionStreams,
		TerminalStore:           s.terminalStore,
		TerminalPersistentStore: s.terminalPersistentStore,
		TerminalBookmarkStore:   s.terminalBookmarkStore,
		SSHKnownHostStore:       s.sshKnownHostStore,
		TerminalScrollbackStore: s.terminalScrollbackStore,
		CredentialStore:         s.credentialStore,
		AuditStore:              s.auditStore,
		LogStore:                s.logStore,
		AssetStore:              s.assetStore,
		GroupStore:              s.groupStore,

		DBPool: dbPoolOrNil(s.db),

		AgentMgr: s.agentMgr,
		JobQueue: s.jobQueue,

		PolicyState:    s.policyState,
		SecretsManager: s.secretsManager,

		TerminalBridges:  &s.terminalBridges,
		PendingAgentCmds: &s.pendingAgentCmds,

		TerminalWebSocketUpgrader: terminalWebSocketUpgrader,

		EnforceRateLimit: s.enforceRateLimit,
		WrapAuth:         s.withAuth,

		IssueStreamTicket: func(ctx context.Context, sessionID string) (string, time.Time, error) {
			return s.issueStreamTicket(ctx, sessionID)
		},
		ConsumeStreamTicketAuth: func(r *http.Request) (*http.Request, bool) {
			return s.consumeStreamTicketAuth(r)
		},
		PrincipalActorID: func(ctx context.Context) string {
			return apiv2.PrincipalActorID(ctx)
		},
		DecodeJSONBody: func(w http.ResponseWriter, r *http.Request, dst any) error {
			return shared.DecodeJSONBody(w, r, dst)
		},
		ValidateMaxLen: func(field, value string, maxLen int) error {
			return shared.ValidateMaxLen(field, value, maxLen)
		},
		AppendAuditEventBestEffort: func(event audit.Event, logMessage string) {
			s.appendAuditEventBestEffort(event, logMessage)
		},
		AppendLogEventBestEffort: func(event logs.Event, logMessage string) {
			s.appendLogEventBestEffort(event, logMessage)
		},
		EvaluateAssetGuardrails: s.ensureGroupFeaturesDeps().EvaluateAssetGuardrails,

		ResolveDockerExecSessionTarget: func(target string) (string, string, bool) {
			return s.ensureDockerDeps().ResolveDockerExecSessionTarget(target)
		},
		HandleDockerExecTerminalStream: func(w http.ResponseWriter, r *http.Request, session terminal.Session, agentID, containerID string) {
			s.ensureDockerDeps().HandleDockerExecTerminalStream(w, r, session, agentID, containerID)
		},
		ResolveProxmoxSessionTarget: func(assetID string) (terminalpkg.ProxmoxSessionTarget, bool, error) {
			target, ok, err := s.ensureProxmoxDeps().ResolveProxmoxSessionTarget(assetID)
			return target, ok, err
		},
		TryProxmoxTerminalStream: func(w http.ResponseWriter, r *http.Request, session terminal.Session, target terminalpkg.ProxmoxSessionTarget) error {
			return s.ensureProxmoxDeps().TryProxmoxTerminalStream(w, r, session, target.(proxmoxpkg.ProxmoxSessionTarget))
		},
		ResolveTrueNASSessionTarget: func(assetID string) (terminalpkg.TruenasShellTarget, bool, error) {
			target, ok, err := s.ensureTruenasDeps().ResolveTrueNASSessionTarget(assetID)
			return target, ok, err
		},
		TryTrueNASTerminalStream: func(w http.ResponseWriter, r *http.Request, session terminal.Session, target terminalpkg.TruenasShellTarget) error {
			return s.ensureTruenasDeps().TryTrueNASTerminalStream(w, r, session, target.(truenaspkg.TruenasShellTarget))
		},

		GetProtocolConfig: func(ctx context.Context, assetID, protocol string) (*protocols.ProtocolConfig, error) {
			if s.db == nil {
				return nil, nil
			}
			return s.db.GetProtocolConfig(ctx, assetID, protocol)
		},

		GetAssetTerminalConfig: func(assetID string) (credentials.AssetTerminalConfig, bool, error) {
			if s.credentialStore == nil {
				return credentials.AssetTerminalConfig{}, false, errors.New("credential store not configured")
			}
			return s.credentialStore.GetAssetTerminalConfig(assetID)
		},
		SaveAssetTerminalConfig: func(cfg credentials.AssetTerminalConfig) (credentials.AssetTerminalConfig, error) {
			if s.credentialStore == nil {
				return credentials.AssetTerminalConfig{}, errors.New("credential store not configured")
			}
			return s.credentialStore.SaveAssetTerminalConfig(cfg)
		},
		HubSSHKeyEnabled: s.hubSSHKeyEnabled,

		TerminalInMemStore:  s.terminalInMemStore,
		EphemeralSSHConfigs: s.ephemeralSSHConfigs,

		ExecuteSSHCommandFn: func(job terminal.CommandJob, mode string, timeout time.Duration, maxOutput int) (string, error) {
			return executeSSHCommand(job, commandExecutorConfig{
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

	return d
}

// ensureTerminalDeps returns the terminal deps, creating and caching on first call.
func (s *apiServer) ensureTerminalDeps() *terminalpkg.Deps {
	s.terminalDepsOnce.Do(func() {
		s.terminalDeps = s.buildTerminalDeps()
	})
	return s.terminalDeps
}

// removeSavedTerminalSessionsForRemovedDevice runs after a device is deleted.
// The device is already gone, so a failure here is logged and the leftover
// saved session can still be removed from the Terminal page. A closed browser
// request must not stop it halfway.
func (s *apiServer) removeSavedTerminalSessionsForRemovedDevice(ctx context.Context, assetID string) {
	if err := s.ensureTerminalDeps().RemovePersistentSessionsForRemovedDevice(context.WithoutCancel(ctx), assetID); err != nil {
		log.Printf("api warning: failed to remove saved terminal sessions of removed device %s: %v", assetID, err) // #nosec G706 -- Asset IDs are Hub records; no secret is logged.
	}
}

// Standalone helper functions that were defined in the terminal session handlers
// and used by other cmd/labtether files.
func canAccessOwnedSession(r *http.Request, sessionActorID string) bool {
	actorID := apiv2.PrincipalActorID(r.Context())
	if apiv2.IsOwnerPrincipal(r.Context()) {
		return true
	}
	return strings.TrimSpace(sessionActorID) == actorID
}
