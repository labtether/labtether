package main

import (
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/auth"
)

type fakeAdminBootstrapStore struct {
	user    auth.User
	exists  bool
	getErr  error
	list    []auth.User
	listErr error

	createErr      error
	createCount    int
	createUser     string
	createHash     string
	bootstrapCount int

	updateErr   error
	updateCount int
	updateID    string
	updateHash  string
}

func (f *fakeAdminBootstrapStore) GetUserByUsername(string) (auth.User, bool, error) {
	if f.getErr != nil {
		return auth.User{}, false, f.getErr
	}
	if !f.exists {
		return auth.User{}, false, nil
	}
	return f.user, true, nil
}

func (f *fakeAdminBootstrapStore) ListUsers(limit int) ([]auth.User, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if limit > 0 && len(f.list) > limit {
		return append([]auth.User(nil), f.list[:limit]...), nil
	}
	return append([]auth.User(nil), f.list...), nil
}

func (f *fakeAdminBootstrapStore) UpdateUserPasswordHash(id, passwordHash string) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updateCount++
	f.updateID = id
	f.updateHash = passwordHash
	return nil
}

func (f *fakeAdminBootstrapStore) CreateUser(username, passwordHash string) (auth.User, error) {
	if f.createErr != nil {
		return auth.User{}, f.createErr
	}
	f.createCount++
	f.createUser = username
	f.createHash = passwordHash
	return auth.User{ID: "usr-admin", Username: username, PasswordHash: passwordHash}, nil
}

func (f *fakeAdminBootstrapStore) BootstrapFirstUser(username, passwordHash string) (auth.User, bool, error) {
	if len(f.list) > 0 {
		return auth.User{}, false, nil
	}
	user, err := f.CreateUser(username, passwordHash)
	if err != nil {
		return auth.User{}, false, err
	}
	f.bootstrapCount++
	f.list = append(f.list, user)
	return user, true, nil
}

func TestBootstrapAdminUserRejectsWeakPasswordInProduction(t *testing.T) {
	t.Setenv("LABTETHER_ENV", "production")
	t.Setenv("LABTETHER_ADMIN_PASSWORD", "password")

	store := &fakeAdminBootstrapStore{}
	err := bootstrapAdminUser(store)
	if err == nil {
		t.Fatalf("expected weak-password rejection error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "weak") {
		t.Fatalf("expected weak-password error, got %v", err)
	}
	if store.createCount != 0 {
		t.Fatalf("expected no user creation on weak password, got createCount=%d", store.createCount)
	}
}

func TestBootstrapAdminUserSkipsAutoCreateInProductionWithoutPassword(t *testing.T) {
	t.Setenv("LABTETHER_ENV", "production")
	t.Setenv("LABTETHER_ADMIN_PASSWORD", "")

	store := &fakeAdminBootstrapStore{}
	if err := bootstrapAdminUser(store); err != nil {
		t.Fatalf("bootstrapAdminUser returned error: %v", err)
	}
	if store.createCount != 0 {
		t.Fatalf("expected no bootstrap user creation, got %d", store.createCount)
	}
}

func TestBootstrapAdminUserCreatesDevDefaultPasswordWhenMissing(t *testing.T) {
	t.Setenv("LABTETHER_ENV", "development")
	t.Setenv("LABTETHER_ADMIN_PASSWORD", "")

	store := &fakeAdminBootstrapStore{}
	if err := bootstrapAdminUser(store); err != nil {
		t.Fatalf("bootstrapAdminUser returned error: %v", err)
	}
	if store.createCount != 1 {
		t.Fatalf("expected one created user, got %d", store.createCount)
	}
	if store.createUser != defaultBootstrapAdminUsername {
		t.Fatalf("expected created username %s, got %s", defaultBootstrapAdminUsername, store.createUser)
	}
	if auth.CheckPassword("password", store.createHash) {
		t.Fatalf("expected generated password, but hash matches weak default")
	}
}

func TestBootstrapAdminUserUsesConfiguredUsername(t *testing.T) {
	t.Setenv("LABTETHER_ENV", "production")
	t.Setenv("LABTETHER_ADMIN_USERNAME", "Captain.Home")
	t.Setenv("LABTETHER_ADMIN_PASSWORD", "strong-password-123")

	store := &fakeAdminBootstrapStore{}
	if err := bootstrapAdminUser(store); err != nil {
		t.Fatalf("bootstrapAdminUser returned error: %v", err)
	}
	if store.createUser != "captain.home" {
		t.Fatalf("expected normalized bootstrap username captain.home, got %s", store.createUser)
	}
}

func TestBootstrapAdminUserRejectsInvalidConfiguredUsername(t *testing.T) {
	t.Setenv("LABTETHER_ENV", "production")
	t.Setenv("LABTETHER_ADMIN_USERNAME", "!!!")
	t.Setenv("LABTETHER_ADMIN_PASSWORD", "strong-password-123")

	store := &fakeAdminBootstrapStore{}
	err := bootstrapAdminUser(store)
	if err == nil {
		t.Fatalf("expected invalid username error")
	}
	if !strings.Contains(err.Error(), "LABTETHER_ADMIN_USERNAME") {
		t.Fatalf("expected LABTETHER_ADMIN_USERNAME error, got %v", err)
	}
}

func TestBootstrapAdminUserDoesNotResetExistingDevAdminPassword(t *testing.T) {
	t.Setenv("LABTETHER_ENV", "development")
	t.Setenv("LABTETHER_ADMIN_PASSWORD", "new-dev-password")

	oldHash, err := auth.HashPassword("old-password")
	if err != nil {
		t.Fatalf("failed to hash old password: %v", err)
	}

	store := &fakeAdminBootstrapStore{
		exists: true,
		list: []auth.User{{
			ID:           "usr-admin",
			Username:     defaultBootstrapAdminUsername,
			PasswordHash: oldHash,
		}},
		user: auth.User{
			ID:           "usr-admin",
			Username:     defaultBootstrapAdminUsername,
			PasswordHash: oldHash,
		},
	}

	if err := bootstrapAdminUser(store); err != nil {
		t.Fatalf("bootstrapAdminUser returned error: %v", err)
	}
	if store.updateCount != 0 {
		t.Fatalf("expected no password-hash update for existing admin, got %d", store.updateCount)
	}
	if store.createCount != 0 {
		t.Fatalf("expected no create call for existing user, got %d", store.createCount)
	}
}

func TestBootstrapAdminUserDoesNotPrintGeneratedPasswordBannerToStderr(t *testing.T) {
	t.Setenv("LABTETHER_ENV", "development")
	t.Setenv("LABTETHER_ADMIN_PASSWORD", "")

	store := &fakeAdminBootstrapStore{}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	defer stderrReader.Close()

	originalStderr := os.Stderr
	originalLogWriter := log.Writer()
	os.Stderr = stderrWriter
	log.SetOutput(io.Discard)
	defer func() {
		os.Stderr = originalStderr
		log.SetOutput(originalLogWriter)
	}()

	if err := bootstrapAdminUser(store); err != nil {
		t.Fatalf("bootstrapAdminUser returned error: %v", err)
	}
	if err := stderrWriter.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	stderrBytes, err := io.ReadAll(stderrReader)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if strings.Contains(string(stderrBytes), "GENERATED ADMIN PASSWORD") {
		t.Fatalf("expected no generated-password banner on stderr, got %q", string(stderrBytes))
	}
}

func TestBootstrapAdminUserReturnsLookupError(t *testing.T) {
	t.Setenv("LABTETHER_ENV", "development")
	t.Setenv("LABTETHER_ADMIN_PASSWORD", "")

	store := &fakeAdminBootstrapStore{listErr: errors.New("db down")}
	err := bootstrapAdminUser(store)
	if err == nil {
		t.Fatalf("expected lookup failure error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "check bootstrap state") {
		t.Fatalf("expected bootstrap-state error, got %v", err)
	}
}

func TestBootstrapAdminUserSkipsWhenAnyUserAlreadyExists(t *testing.T) {
	t.Setenv("LABTETHER_ENV", "production")
	t.Setenv("LABTETHER_ADMIN_USERNAME", "second.owner")
	t.Setenv("LABTETHER_ADMIN_PASSWORD", "strong-password-123")

	store := &fakeAdminBootstrapStore{
		list: []auth.User{{ID: "usr-existing", Username: "first.owner", Role: auth.RoleOwner}},
	}
	if err := bootstrapAdminUser(store); err != nil {
		t.Fatalf("bootstrapAdminUser returned error: %v", err)
	}
	if store.createCount != 0 {
		t.Fatalf("expected no bootstrap user creation when an owner already exists, got %d", store.createCount)
	}
	if store.bootstrapCount != 0 {
		t.Fatalf("expected no bootstrap-first-user call when users already exist, got %d", store.bootstrapCount)
	}
}

func TestIsBootstrapAdminUsernameUsesConfiguredValue(t *testing.T) {
	t.Setenv("LABTETHER_ADMIN_USERNAME", "Owner.One")
	if !isBootstrapAdminUsername("owner.one") {
		t.Fatalf("expected configured bootstrap username match")
	}
	if isBootstrapAdminUsername("admin") {
		t.Fatalf("did not expect default admin alias once custom bootstrap username is configured")
	}
}

func TestAuthBootstrapSetupRequired(t *testing.T) {
	store := &fakeAdminBootstrapStore{}
	required, err := authBootstrapSetupRequired(store)
	if err != nil {
		t.Fatalf("authBootstrapSetupRequired returned error: %v", err)
	}
	if !required {
		t.Fatalf("expected setup required when no users exist")
	}

	store.list = []auth.User{{ID: "usr-1", Username: "owner.one"}}
	required, err = authBootstrapSetupRequired(store)
	if err != nil {
		t.Fatalf("authBootstrapSetupRequired returned error: %v", err)
	}
	if required {
		t.Fatalf("did not expect setup required when users exist")
	}
}
