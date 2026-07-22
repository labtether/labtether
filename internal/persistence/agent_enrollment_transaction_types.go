package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/enrollment"
)

var (
	ErrEnrollmentTokenInvalid                 = errors.New("enrollment token is invalid, expired, revoked, or exhausted")
	ErrRecoveryRequiresSingleUseToken         = errors.New("identity recovery requires a single-use enrollment token")
	ErrAgentIdentityContinuityConflict        = errors.New("agent identity continuity check failed")
	ErrAgentIdentityProofV2Required           = errors.New("agent identity proof v2 is required for recovery")
	ErrEnrollmentTokenPredatesRotation        = errors.New("enrollment token predates the latest agent credential rotation")
	ErrAgentFleetCapacityReached              = errors.New("agent enrollment capacity reached")
	ErrPreparedAgentApprovalNotFound          = errors.New("prepared agent approval not found")
	ErrAgentApprovalAssetConflict             = errors.New("an asset with the approved stable id already exists")
	ErrAgentCredentialInactive                = errors.New("agent credential is inactive or does not match the asset")
	ErrAgentEnrollmentTransactionsUnavailable = errors.New("agent enrollment transactions are unavailable")
)

// AgentEnrollmentCommitRequest contains only server-validated identity data.
// Raw proof fields never cross the persistence boundary.
type AgentEnrollmentCommitRequest struct {
	AssetID             string
	Hostname            string
	Platform            string
	GroupID             string
	EnrollmentTokenHash string
	AgentTokenHash      string
	AgentTokenExpiresAt time.Time
	DeviceFingerprint   string
	DeviceKeyAlgorithm  string
	DeviceProofVersion  string
	MaxEnrolledAgents   int
}

type AgentEnrollmentCommitResult struct {
	EnrollmentToken enrollment.EnrollmentToken
	AgentToken      enrollment.AgentToken
	Asset           assets.Asset
	Recovery        bool
}

type AgentApprovalPrepareRequest struct {
	AssetID                string
	AgentTokenHash         string
	PreparedTokenExpiresAt time.Time
	MaxEnrolledAgents      int
}

type AgentApprovalFinalizeRequest struct {
	PreparedTokenID     string
	AssetID             string
	Hostname            string
	Platform            string
	DeviceFingerprint   string
	DeviceKeyAlgorithm  string
	AgentTokenExpiresAt time.Time
}

// AgentEnrollmentTransactionStore owns all state transitions that must be
// serialized with the immutable device anchor and per-agent bearer lifecycle.
type AgentEnrollmentTransactionStore interface {
	CommitAgentEnrollment(ctx context.Context, req AgentEnrollmentCommitRequest) (AgentEnrollmentCommitResult, error)
	PrepareAgentApproval(ctx context.Context, req AgentApprovalPrepareRequest) (enrollment.AgentToken, error)
	FinalizeAgentApproval(ctx context.Context, req AgentApprovalFinalizeRequest) (assets.Asset, error)
	CancelAgentApproval(ctx context.Context, preparedTokenID string) error
	DecommissionAgentAsset(ctx context.Context, assetID string) error
	ValidateActiveAgentTokenID(ctx context.Context, agentTokenID, assetID string) error
	CommitAuthenticatedAgentHeartbeat(ctx context.Context, agentTokenID string, req assets.HeartbeatRequest) (assets.Asset, error)
	CommitExistingOwnerAgentHeartbeat(ctx context.Context, req assets.HeartbeatRequest) (assets.Asset, error)
}
