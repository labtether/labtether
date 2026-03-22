package auth

import (
	"errors"
	"time"
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSessionExpired     = errors.New("session expired")
	ErrSessionNotFound    = errors.New("session not found")
)

type User struct {
	ID                string     `json:"id"`
	Username          string     `json:"username"`
	Role              string     `json:"role"`
	AuthProvider      string     `json:"auth_provider"`
	OIDCSubject       string     `json:"oidc_subject,omitempty"`
	PasswordHash      string     `json:"-"`
	TOTPSecret        string     `json:"-"`
	TOTPVerifiedAt    *time.Time `json:"-"`
	TOTPRecoveryCodes string     `json:"-"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type UserInfo struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

const (
	RoleOwner    = "owner"
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

func IsValidRole(role string) bool {
	switch role {
	case RoleOwner, RoleAdmin, RoleOperator, RoleViewer:
		return true
	default:
		return false
	}
}

func IsReadOnlyRole(role string) bool {
	return role == RoleViewer
}
