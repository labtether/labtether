package terminal

import "time"

type Bookmark struct {
	ID                  string     `json:"id"`
	ActorID             string     `json:"actor_id"`
	Title               string     `json:"title"`
	AssetID             string     `json:"asset_id,omitempty"`
	Host                string     `json:"host,omitempty"`
	Port                *int       `json:"port,omitempty"`
	Username            string     `json:"username,omitempty"`
	CredentialProfileID string     `json:"credential_profile_id,omitempty"`
	JumpChainGroupID    string     `json:"jump_chain_group_id,omitempty"`
	Tags                []string   `json:"tags"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	LastUsedAt          *time.Time `json:"last_used_at,omitempty"`
}

type CreateBookmarkRequest struct {
	ActorID             string   `json:"actor_id"`
	Title               string   `json:"title"`
	AssetID             string   `json:"asset_id,omitempty"`
	Host                string   `json:"host,omitempty"`
	Port                *int     `json:"port,omitempty"`
	Username            string   `json:"username,omitempty"`
	CredentialProfileID string   `json:"credential_profile_id,omitempty"`
	JumpChainGroupID    string   `json:"jump_chain_group_id,omitempty"`
	Tags                []string `json:"tags,omitempty"`
}

type UpdateBookmarkRequest struct {
	Title               *string  `json:"title,omitempty"`
	AssetID             *string  `json:"asset_id,omitempty"`
	Host                *string  `json:"host,omitempty"`
	Port                *int     `json:"port,omitempty"`
	Username            *string  `json:"username,omitempty"`
	CredentialProfileID *string  `json:"credential_profile_id,omitempty"`
	JumpChainGroupID    *string  `json:"jump_chain_group_id,omitempty"`
	Tags                []string `json:"tags,omitempty"`
}
