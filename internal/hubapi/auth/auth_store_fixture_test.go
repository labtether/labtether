package auth

import (
	"fmt"
	"github.com/labtether/labtether/internal/auth"
	"sync"
	"time"
)

// --- in-memory AuthStore for tests ---

type memAuthStore struct {
	mu       sync.Mutex
	users    []auth.User
	sessions []auth.Session
	nextID   int
}

func newMemAuthStore() *memAuthStore {
	return &memAuthStore{users: []auth.User{}, sessions: []auth.Session{}}
}

func (s *memAuthStore) genID() string {
	s.nextID++
	return "id-" + string(rune('0'+s.nextID))
}

func (s *memAuthStore) GetUserByID(id string) (auth.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.ID == id {
			return u, true, nil
		}
	}
	return auth.User{}, false, nil
}

func (s *memAuthStore) GetUserByUsername(username string) (auth.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.Username == username {
			return u, true, nil
		}
	}
	return auth.User{}, false, nil
}

func (s *memAuthStore) GetUserByOIDCIdentity(provider, issuer, subject string) (auth.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.AuthProvider == provider && u.OIDCIssuer == issuer && u.OIDCSubject == subject {
			return u, true, nil
		}
	}
	return auth.User{}, false, nil
}

func (s *memAuthStore) GetUserByOIDCSubject(provider, subject string) (auth.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.AuthProvider == provider && u.OIDCIssuer == "" && u.OIDCSubject == subject {
			return u, true, nil
		}
	}
	return auth.User{}, false, nil
}

func (s *memAuthStore) ListUsers(limit int) ([]auth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.users) {
		limit = len(s.users)
	}
	return append([]auth.User{}, s.users[:limit]...), nil
}

func (s *memAuthStore) BootstrapFirstUser(username, passwordHash string) (auth.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.users) > 0 {
		return s.users[0], false, nil
	}
	u := auth.User{
		ID: s.genID(), Username: username, PasswordHash: passwordHash,
		Role: auth.RoleOwner, AuthProvider: "local",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	s.users = append(s.users, u)
	return u, true, nil
}

func (s *memAuthStore) CreateUser(username, passwordHash string) (auth.User, error) {
	return s.CreateUserWithRole(username, passwordHash, auth.RoleViewer, "local", "")
}

func (s *memAuthStore) CreateUserWithRole(username, passwordHash, role, provider, oidcSubject string) (auth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := auth.User{
		ID: s.genID(), Username: username, PasswordHash: passwordHash,
		Role: role, AuthProvider: provider, OIDCSubject: oidcSubject,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	s.users = append(s.users, u)
	return u, nil
}

func (s *memAuthStore) CreateUserWithOIDCIdentity(
	username, passwordHash, role, provider, issuer, subject string,
) (auth.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.users {
		if existing.Username == username {
			return auth.User{}, fmt.Errorf("user %q already exists", username)
		}
		if existing.AuthProvider == provider && existing.OIDCIssuer == issuer && existing.OIDCSubject == subject {
			return auth.User{}, fmt.Errorf("oidc identity already exists")
		}
	}
	u := auth.User{
		ID: s.genID(), Username: username, PasswordHash: passwordHash,
		Role: role, AuthProvider: provider, OIDCIssuer: issuer, OIDCSubject: subject,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	s.users = append(s.users, u)
	return u, nil
}

func (s *memAuthStore) BindLegacyOIDCIdentity(
	id, provider, subject, issuer string,
) (auth.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.users {
		if existing.ID != id && existing.AuthProvider == provider && existing.OIDCIssuer == issuer && existing.OIDCSubject == subject {
			return auth.User{}, false, fmt.Errorf("oidc identity already exists")
		}
	}
	for i := range s.users {
		if s.users[i].ID == id && s.users[i].AuthProvider == provider && s.users[i].OIDCIssuer == "" && s.users[i].OIDCSubject == subject {
			s.users[i].OIDCIssuer = issuer
			s.users[i].UpdatedAt = time.Now().UTC()
			return s.users[i], true, nil
		}
	}
	return auth.User{}, false, nil
}

func (s *memAuthStore) UpdateUserPasswordHash(id, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.users {
		if s.users[i].ID == id {
			s.users[i].PasswordHash = hash
			return nil
		}
	}
	return nil
}

func (s *memAuthStore) UpdateUserRole(id, role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.users {
		if s.users[i].ID == id {
			s.users[i].Role = role
			return nil
		}
	}
	return nil
}

func (s *memAuthStore) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.users {
		if s.users[i].ID == id {
			s.users[i] = s.users[len(s.users)-1]
			s.users = s.users[:len(s.users)-1]
			return nil
		}
	}
	return nil
}

func (s *memAuthStore) ListSessionsByUserID(userID string) ([]auth.Session, error) {
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

func (s *memAuthStore) SetUserTOTPSecret(id, secret string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.users {
		if s.users[i].ID == id {
			s.users[i].TOTPSecret = secret
			return nil
		}
	}
	return nil
}

func (s *memAuthStore) ConfirmUserTOTP(id, codes string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for i := range s.users {
		if s.users[i].ID == id {
			s.users[i].TOTPVerifiedAt = &now
			s.users[i].TOTPRecoveryCodes = codes
			return nil
		}
	}
	return nil
}

func (s *memAuthStore) ClearUserTOTP(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.users {
		if s.users[i].ID == id {
			s.users[i].TOTPSecret = ""
			s.users[i].TOTPVerifiedAt = nil
			s.users[i].TOTPRecoveryCodes = ""
			return nil
		}
	}
	return nil
}

func (s *memAuthStore) UpdateUserRecoveryCodes(id, codes string) error { return nil }

func (s *memAuthStore) ConsumeRecoveryCode(userID, code string) (bool, error) {
	return false, nil
}

func (s *memAuthStore) CreateAuthSession(userID, tokenHash string, expiresAt time.Time) (auth.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := auth.Session{
		ID: s.genID(), UserID: userID, TokenHash: tokenHash,
		ExpiresAt: expiresAt, CreatedAt: time.Now().UTC(),
	}
	s.sessions = append(s.sessions, sess)
	return sess, nil
}

func (s *memAuthStore) ValidateSession(tokenHash string) (auth.Session, bool, error) {
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

func (s *memAuthStore) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.sessions {
		if s.sessions[i].ID == id {
			s.sessions[i] = s.sessions[len(s.sessions)-1]
			s.sessions = s.sessions[:len(s.sessions)-1]
			return nil
		}
	}
	return nil
}

func (s *memAuthStore) DeleteSessionsByUserID(userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var kept []auth.Session
	for _, sess := range s.sessions {
		if sess.UserID != userID {
			kept = append(kept, sess)
		}
	}
	s.sessions = kept
	return nil
}

func (s *memAuthStore) DeleteExpiredSessions() (int64, error) { return 0, nil }
