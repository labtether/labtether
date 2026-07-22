package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

const (
	bootstrapSetupTokenHeader = "X-Labtether-Setup-Token" // #nosec G101 -- This is a public HTTP header name, not a credential value.
	maxBootstrapSetupTokenLen = 512
)

var (
	ErrBootstrapSetupTokenNotConfigured = errors.New("bootstrap setup token is not configured")
	ErrBootstrapSetupTokenInvalid       = errors.New("bootstrap setup token is invalid")
)

// BootstrapSetupTokenHeader returns the header used to present the one-time
// local setup token. Keeping the token out of the JSON body reduces the chance
// that generic request-body diagnostics capture it.
func BootstrapSetupTokenHeader() string {
	return bootstrapSetupTokenHeader
}

// ValidateBootstrapSetupToken validates the caller-provided first-run setup
// token against a dedicated secret. The service owner/API token is deliberately
// not accepted here: the browser-facing console must never be able to turn its
// internal service credential into an unauthenticated owner bootstrap.
func ValidateBootstrapSetupToken(provided string) error {
	expected, err := configuredBootstrapSetupToken()
	if err != nil {
		return err
	}
	provided = strings.TrimSpace(provided)
	if provided == "" || len(provided) > maxBootstrapSetupTokenLen || len(provided) != len(expected) {
		return ErrBootstrapSetupTokenInvalid
	}
	if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		return ErrBootstrapSetupTokenInvalid
	}
	return nil
}

func configuredBootstrapSetupToken() (string, error) {
	if path := strings.TrimSpace(os.Getenv("LABTETHER_SETUP_TOKEN_FILE")); path != "" {
		return readBootstrapSetupTokenFile(path)
	}
	if token := strings.TrimSpace(os.Getenv("LABTETHER_SETUP_TOKEN")); token != "" && len(token) <= maxBootstrapSetupTokenLen {
		return token, nil
	}
	path := filepath.Join(shared.EnvOrDefault("LABTETHER_DATA_DIR", "data"), "install", "setup-token")
	if token, err := readBootstrapSetupTokenFile(path); err == nil {
		return token, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("%w: create setup token directory", ErrBootstrapSetupTokenNotConfigured)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("%w: generate setup token", ErrBootstrapSetupTokenNotConfigured)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	root, name, err := openRootedFileParent(path)
	if err != nil {
		return "", fmt.Errorf("%w: open setup token directory", ErrBootstrapSetupTokenNotConfigured)
	}
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		_ = root.Close()
		return readBootstrapSetupTokenFile(path)
	}
	if err != nil {
		_ = root.Close()
		return "", fmt.Errorf("%w: create setup token file", ErrBootstrapSetupTokenNotConfigured)
	}
	if _, err := file.WriteString(token); err != nil {
		_ = file.Close()
		_ = root.Remove(name)
		_ = root.Close()
		return "", fmt.Errorf("%w: write setup token file", ErrBootstrapSetupTokenNotConfigured)
	}
	if err := file.Close(); err != nil {
		_ = root.Remove(name)
		_ = root.Close()
		return "", fmt.Errorf("%w: close setup token file", ErrBootstrapSetupTokenNotConfigured)
	}
	if err := root.Close(); err != nil {
		return "", fmt.Errorf("%w: close setup token directory", ErrBootstrapSetupTokenNotConfigured)
	}
	return token, nil
}

func readBootstrapSetupTokenFile(path string) (string, error) {
	root, name, err := openRootedFileParent(path)
	if err != nil {
		return "", fmt.Errorf("%w: open setup token directory", ErrBootstrapSetupTokenNotConfigured)
	}
	defer func() { _ = root.Close() }()

	raw, err := root.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("%w: read setup token file", ErrBootstrapSetupTokenNotConfigured)
	}
	if token := strings.TrimSpace(string(raw)); token != "" && len(token) <= maxBootstrapSetupTokenLen {
		return token, nil
	}
	return "", ErrBootstrapSetupTokenNotConfigured
}

// openRootedFileParent converts an operator-configured file path into an
// os.Root plus a single basename. File operations then cannot follow a symlink
// outside the selected parent directory or traverse via the final component.
func openRootedFileParent(path string) (*os.Root, string, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	dir, name := filepath.Split(clean)
	if clean == "." || name == "" || name == "." || name == ".." {
		return nil, "", errors.New("invalid setup token file path")
	}
	if dir == "" {
		dir = "."
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, "", err
	}
	return root, name, nil
}

// ConsumeBootstrapSetupToken removes a file-backed setup token after the
// atomic first-user transaction succeeds. An explicitly supplied environment
// token cannot be erased from the process environment, but remains unusable
// because BootstrapFirstUser permits only the initial owner.
func ConsumeBootstrapSetupToken() {
	if strings.TrimSpace(os.Getenv("LABTETHER_SETUP_TOKEN")) != "" && strings.TrimSpace(os.Getenv("LABTETHER_SETUP_TOKEN_FILE")) == "" {
		return
	}
	path := strings.TrimSpace(os.Getenv("LABTETHER_SETUP_TOKEN_FILE"))
	if path == "" {
		path = filepath.Join(shared.EnvOrDefault("LABTETHER_DATA_DIR", "data"), "install", "setup-token")
	}
	root, name, err := openRootedFileParent(path)
	if err != nil {
		securityruntime.Logf("labtether auth: WARNING: failed to open consumed setup token directory %s: %v", path, err)
		return
	}
	defer func() { _ = root.Close() }()
	if err := root.Remove(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		securityruntime.Logf("labtether auth: WARNING: failed to remove consumed setup token file %s: %v", path, err)
	}
}

func bootstrapSetupTokenLocation() string {
	if path := strings.TrimSpace(os.Getenv("LABTETHER_SETUP_TOKEN_FILE")); path != "" {
		return path
	}
	if strings.TrimSpace(os.Getenv("LABTETHER_SETUP_TOKEN")) != "" {
		return "LABTETHER_SETUP_TOKEN"
	}
	return filepath.Join(shared.EnvOrDefault("LABTETHER_DATA_DIR", "data"), "install", "setup-token")
}

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
			if _, err := configuredBootstrapSetupToken(); err != nil {
				return fmt.Errorf("prepare first-run setup token: %w", err)
			}
			securityruntime.Logf("labtether auth: no bootstrap admin password configured; first-run setup token is available locally at %s", bootstrapSetupTokenLocation())
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
