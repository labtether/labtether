package persistence

import (
	"fmt"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/auth"
)

// MemoryAuthStore provides an in-memory implementation of AuthStore for tests.
type MemoryAuthStore struct {
	mu       sync.Mutex
	users    []auth.User
	sessions []auth.Session
	nextID   int
}

// NewMemoryAuthStore returns a new MemoryAuthStore.
func NewMemoryAuthStore() *MemoryAuthStore {
	return &MemoryAuthStore{}
}

func (s *MemoryAuthStore) nextUserID() string {
	s.nextID++
	return fmt.Sprintf("user-%d", s.nextID)
}

func (s *MemoryAuthStore) nextSessionID() string {
	s.nextID++
	return fmt.Sprintf("session-%d", s.nextID)
}

func (s *MemoryAuthStore) GetUserByID(id string) (auth.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.ID == id {
			return u, true, nil
		}
	}
	return auth.User{}, false, nil
}

func (s *MemoryAuthStore) GetUserByUsername(username string) (auth.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.Username == username {
			return u, true, nil
		}
	}
	return auth.User{}, false, nil
}

func (s *MemoryAuthStore) GetUserByOIDCSubject(provider, subject string) (auth.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.AuthProvider == provider && u.OIDCSubject == subject {
			return u, true, nil
		}
	}
	return auth.User{}, false, nil
}

func (s *MemoryAuthStore) ListUsers(limit int) ([]auth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.users) {
		limit = len(s.users)
	}
	out := make([]auth.User, limit)
	copy(out, s.users[:limit])
	return out, nil
}

func (s *MemoryAuthStore) BootstrapFirstUser(username, passwordHash string) (auth.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.users) > 0 {
		return auth.User{}, false, nil
	}
	now := time.Now().UTC()
	u := auth.User{
		ID:           s.nextUserID(),
		Username:     username,
		PasswordHash: passwordHash,
		Role:         auth.RoleOwner,
		AuthProvider: "local",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.users = append(s.users, u)
	return u, true, nil
}

func (s *MemoryAuthStore) CreateUser(username, passwordHash string) (auth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.Username == username {
			return auth.User{}, fmt.Errorf("user %q already exists", username)
		}
	}
	now := time.Now().UTC()
	u := auth.User{
		ID:           s.nextUserID(),
		Username:     username,
		PasswordHash: passwordHash,
		Role:         auth.RoleOwner,
		AuthProvider: "local",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.users = append(s.users, u)
	return u, nil
}

func (s *MemoryAuthStore) CreateUserWithRole(username, passwordHash, role, authProvider, oidcSubject string) (auth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.Username == username {
			return auth.User{}, fmt.Errorf("user %q already exists", username)
		}
	}
	now := time.Now().UTC()
	u := auth.User{
		ID:           s.nextUserID(),
		Username:     username,
		PasswordHash: passwordHash,
		Role:         role,
		AuthProvider: authProvider,
		OIDCSubject:  oidcSubject,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.users = append(s.users, u)
	return u, nil
}

func (s *MemoryAuthStore) UpdateUserPasswordHash(id, passwordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, u := range s.users {
		if u.ID == id {
			s.users[i].PasswordHash = passwordHash
			s.users[i].UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return auth.ErrUserNotFound
}

func (s *MemoryAuthStore) UpdateUserRole(id, role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, u := range s.users {
		if u.ID == id {
			s.users[i].Role = role
			s.users[i].UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return auth.ErrUserNotFound
}

func (s *MemoryAuthStore) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, u := range s.users {
		if u.ID == id {
			s.users = append(s.users[:i], s.users[i+1:]...)
			return nil
		}
	}
	return auth.ErrUserNotFound
}

func (s *MemoryAuthStore) ListSessionsByUserID(userID string) ([]auth.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []auth.Session
	for _, sess := range s.sessions {
		if sess.UserID == userID {
			out = append(out, sess)
		}
	}
	return out, nil
}

func (s *MemoryAuthStore) SetUserTOTPSecret(id, encryptedSecret string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, u := range s.users {
		if u.ID == id {
			s.users[i].TOTPSecret = encryptedSecret
			s.users[i].UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return auth.ErrUserNotFound
}

func (s *MemoryAuthStore) ConfirmUserTOTP(id, recoveryCodes string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, u := range s.users {
		if u.ID == id {
			now := time.Now().UTC()
			s.users[i].TOTPVerifiedAt = &now
			s.users[i].TOTPRecoveryCodes = recoveryCodes
			s.users[i].UpdatedAt = now
			return nil
		}
	}
	return auth.ErrUserNotFound
}

func (s *MemoryAuthStore) ClearUserTOTP(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, u := range s.users {
		if u.ID == id {
			s.users[i].TOTPSecret = ""
			s.users[i].TOTPVerifiedAt = nil
			s.users[i].TOTPRecoveryCodes = ""
			s.users[i].UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return auth.ErrUserNotFound
}

func (s *MemoryAuthStore) UpdateUserRecoveryCodes(id, recoveryCodes string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, u := range s.users {
		if u.ID == id {
			s.users[i].TOTPRecoveryCodes = recoveryCodes
			s.users[i].UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return auth.ErrUserNotFound
}

func (s *MemoryAuthStore) ConsumeRecoveryCode(userID, code string) (bool, error) {
	// Simplified: always returns false for in-memory tests.
	return false, nil
}

func (s *MemoryAuthStore) CreateAuthSession(userID, tokenHash string, expiresAt time.Time) (auth.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	sess := auth.Session{
		ID:        s.nextSessionID(),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}
	s.sessions = append(s.sessions, sess)
	return sess, nil
}

func (s *MemoryAuthStore) ValidateSession(tokenHash string) (auth.Session, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for _, sess := range s.sessions {
		if sess.TokenHash == tokenHash && sess.ExpiresAt.After(now) {
			return sess, true, nil
		}
	}
	return auth.Session{}, false, nil
}

func (s *MemoryAuthStore) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, sess := range s.sessions {
		if sess.ID == id {
			s.sessions = append(s.sessions[:i], s.sessions[i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *MemoryAuthStore) DeleteSessionsByUserID(userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var remaining []auth.Session
	for _, sess := range s.sessions {
		if sess.UserID != userID {
			remaining = append(remaining, sess)
		}
	}
	s.sessions = remaining
	return nil
}

func (s *MemoryAuthStore) DeleteExpiredSessions() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	var remaining []auth.Session
	var deleted int64
	for _, sess := range s.sessions {
		if sess.ExpiresAt.After(now) {
			remaining = append(remaining, sess)
		} else {
			deleted++
		}
	}
	s.sessions = remaining
	return deleted, nil
}
