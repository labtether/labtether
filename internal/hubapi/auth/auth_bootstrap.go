package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/securityruntime"
)

// WeakPasswords that must never be used in production.
var WeakPasswords = map[string]bool{
	"admin":     true,
	"password":  true,
	"123456":    true,
	"labtether": true,
}

// DefaultBootstrapAdminUsername is the default admin username.
const DefaultBootstrapAdminUsername = "admin"

// AdminBootstrapStore is the interface required for admin bootstrapping.
type AdminBootstrapStore interface {
	GetUserByUsername(username string) (auth.User, bool, error)
	ListUsers(limit int) ([]auth.User, error)
	BootstrapFirstUser(username, passwordHash string) (auth.User, bool, error)
	UpdateUserPasswordHash(id, passwordHash string) error
	CreateUser(username, passwordHash string) (auth.User, error)
}

// BootstrapAdminUser creates the initial admin user if none exists.
func BootstrapAdminUser(store AdminBootstrapStore) error {
	if store == nil {
		return errors.New("auth store is required")
	}

	env := strings.ToLower(shared.EnvOrDefault("LABTETHER_ENV", "development"))
	isProd := env == "production" || env == "prod"
	adminUsername, err := ConfiguredBootstrapAdminUsername()
	if err != nil {
		return err
	}
	configuredPassword := shared.EnvOrDefault("LABTETHER_ADMIN_PASSWORD", "")

	setupRequired, err := AuthBootstrapSetupRequired(store)
	if err != nil {
		return fmt.Errorf("check bootstrap state: %w", err)
	}
	if !setupRequired {
		return nil
	}

	password := configuredPassword
	generated := false

	if password == "" {
		if isProd {
			securityruntime.Logf("labtether auth: no bootstrap admin password configured in production; waiting for post-install setup flow")
			return nil
		}
		// Generate a strong random password when none is configured.
		b := make([]byte, 24)
		if _, err := rand.Read(b); err != nil {
			return fmt.Errorf("generate random admin password: %w", err)
		}
		password = base64.RawURLEncoding.EncodeToString(b)
		generated = true
	}

	// Block weak passwords in production.
	if isProd && WeakPasswords[password] {
		return fmt.Errorf("refusing weak admin password in production")
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	user, created, err := store.BootstrapFirstUser(adminUsername, hash)
	if err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}
	if !created {
		return nil
	}

	if generated {
		securityruntime.Logf("labtether auth: created bootstrap admin user %q (id=%s) with a generated password that was intentionally not logged; set LABTETHER_ADMIN_USERNAME/LABTETHER_ADMIN_PASSWORD to control this", user.Username, user.ID)
	} else if isProd {
		securityruntime.Logf("labtether auth: created bootstrap admin user %q (id=%s) from LABTETHER_ADMIN_USERNAME/LABTETHER_ADMIN_PASSWORD", user.Username, user.ID)
	} else {
		securityruntime.Logf("labtether auth: created bootstrap admin user %q (id=%s) with an explicitly configured dev password", user.Username, user.ID)
	}
	return nil
}

// ConfiguredBootstrapAdminUsername returns the configured bootstrap admin username.
func ConfiguredBootstrapAdminUsername() (string, error) {
	raw := shared.EnvOrDefault("LABTETHER_ADMIN_USERNAME", DefaultBootstrapAdminUsername)
	username := NormalizeUsername(raw)
	if username == "" {
		return "", errors.New("LABTETHER_ADMIN_USERNAME must contain at least one valid username character")
	}
	return username, nil
}

// IsBootstrapAdminUsername checks if the given username matches the configured bootstrap admin.
func IsBootstrapAdminUsername(username string) bool {
	configured, err := ConfiguredBootstrapAdminUsername()
	if err != nil {
		return strings.EqualFold(strings.TrimSpace(username), DefaultBootstrapAdminUsername)
	}
	return strings.EqualFold(strings.TrimSpace(username), configured)
}

// AuthBootstrapSetupRequired checks if the initial bootstrap setup is needed.
func AuthBootstrapSetupRequired(store AdminBootstrapStore) (bool, error) {
	if store == nil {
		return false, errors.New("auth store is required")
	}
	users, err := store.ListUsers(1)
	if err != nil {
		return false, fmt.Errorf("list users: %w", err)
	}
	return len(users) == 0, nil
}
