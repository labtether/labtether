package terminal

import (
	"time"

	"golang.org/x/crypto/ssh"
)

// Session represents an active terminal session request.
type Session struct {
	ID                  string     `json:"id"`
	ActorID             string     `json:"actor_id"`
	Target              string     `json:"target"`
	Mode                string     `json:"mode"`
	Status              string     `json:"status"`
	Source              string     `json:"source,omitempty"`
	PersistentSessionID string     `json:"persistent_session_id,omitempty"`
	TmuxSessionName     string     `json:"tmux_session_name,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	LastActionAt        time.Time  `json:"last_action_at"`
	InlineSSHConfig     *SSHConfig `json:"-"`
}

// Command captures one command request inside a session.
type Command struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	ActorID   string    `json:"actor_id"`
	Target    string    `json:"target"`
	Body      string    `json:"body"`
	Mode      string    `json:"mode"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Output    string    `json:"output,omitempty"`
}

type CreateSessionRequest struct {
	ActorID             string `json:"actor_id"`
	Target              string `json:"target"`
	Mode                string `json:"mode"`
	PersistentSessionID string `json:"persistent_session_id,omitempty"`
	TmuxSessionName     string `json:"tmux_session_name,omitempty"`
}

type PersistentSession struct {
	ID               string     `json:"id"`
	ActorID          string     `json:"actor_id"`
	Target           string     `json:"target"`
	Title            string     `json:"title"`
	Status           string     `json:"status"`
	TmuxSessionName  string     `json:"tmux_session_name"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	LastAttachedAt   *time.Time `json:"last_attached_at,omitempty"`
	LastDetachedAt   *time.Time `json:"last_detached_at,omitempty"`
	BookmarkID       string     `json:"bookmark_id,omitempty"`
	ArchivedAt       *time.Time `json:"archived_at,omitempty"`
	ArchiveAfterDays *int       `json:"archive_after_days,omitempty"`
	Pinned           bool       `json:"pinned"`
}

type CreatePersistentSessionRequest struct {
	ActorID    string `json:"actor_id"`
	Target     string `json:"target"`
	Title      string `json:"title"`
	BookmarkID string `json:"bookmark_id,omitempty"`
}

type UpdatePersistentSessionRequest struct {
	Title  *string `json:"title,omitempty"`
	Status *string `json:"status,omitempty"`
}

type CreateCommandRequest struct {
	ActorID string `json:"actor_id"`
	Command string `json:"command"`
}

type CommandJob struct {
	JobID       string     `json:"job_id"`
	SessionID   string     `json:"session_id"`
	CommandID   string     `json:"command_id"`
	ActorID     string     `json:"actor_id"`
	Target      string     `json:"target"`
	Command     string     `json:"command"`
	Mode        string     `json:"mode"`
	TimeoutSec  int        `json:"timeout_sec,omitempty"`
	SSHConfig   *SSHConfig `json:"ssh_config,omitempty"`
	RequestedAt time.Time  `json:"requested_at"`
}

type SSHConfig struct {
	Host                 string `json:"host"`
	Port                 int    `json:"port,omitempty"`
	User                 string `json:"user"`
	Password             string `json:"password,omitempty"`               // #nosec G117 -- Session request carries runtime auth material.
	PrivateKey           string `json:"private_key,omitempty"`            // #nosec G117 -- Session request carries runtime auth material.
	PrivateKeyPassphrase string `json:"private_key_passphrase,omitempty"` // #nosec G117 -- Session request carries runtime auth material.
	StrictHostKey        bool   `json:"strict_host_key,omitempty"`
	HostKey              string `json:"host_key,omitempty"`
}

// HopConfig represents a single hop in an SSH jump chain.
type HopConfig struct {
	Host                string `json:"host"`
	Port                int    `json:"port"`
	Username            string `json:"username"`
	CredentialProfileID string `json:"credential_profile_id"`
}

// JumpChain is an ordered list of SSH hops to traverse before reaching the target.
type JumpChain struct {
	Hops []HopConfig `json:"hops"`
}

// ResolvedHop is a HopConfig with credentials pre-resolved into an ssh.ClientConfig.
type ResolvedHop struct {
	Addr         string
	ClientConfig *ssh.ClientConfig
}

type CommandResult struct {
	JobID       string    `json:"job_id"`
	SessionID   string    `json:"session_id"`
	CommandID   string    `json:"command_id"`
	Status      string    `json:"status"`
	Output      string    `json:"output"`
	CompletedAt time.Time `json:"completed_at"`
}
