package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const ChallengeTTL = 5 * time.Minute

type challenge struct {
	UserID    string
	ExpiresAt time.Time
}

type ChallengeStore struct {
	mu         sync.Mutex
	challenges map[string]challenge
	users      map[string]*userAuthenticationLock
}

type userAuthenticationLock struct {
	mu   sync.Mutex
	refs int
}

func NewChallengeStore() *ChallengeStore {
	return &ChallengeStore{challenges: make(map[string]challenge)}
}

// LockUser orders local password/challenge login against credential changes and
// session revocation. Hold it from the fresh credential read through challenge
// issuance or session creation, or through the revocation and credential write.
// This lock, like the challenges it protects, belongs to this Hub process only.
func (s *ChallengeStore) LockUser(userID string) func() {
	s.mu.Lock()
	if s.users == nil {
		s.users = make(map[string]*userAuthenticationLock)
	}
	entry := s.users[userID]
	if entry == nil {
		entry = &userAuthenticationLock{}
		s.users[userID] = entry
	}
	entry.refs++
	s.mu.Unlock()
	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		s.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(s.users, userID)
		}
		s.mu.Unlock()
	}
}

func (s *ChallengeStore) Create(userID string) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.challenges[token] = challenge{
		UserID:    userID,
		ExpiresAt: time.Now().Add(ChallengeTTL),
	}
	return token
}

// Validate checks a challenge token without consuming it.
func (s *ChallengeStore) Validate(token string) (userID string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, exists := s.challenges[token]
	if !exists || time.Now().After(ch.ExpiresAt) {
		delete(s.challenges, token)
		return "", false
	}
	return ch.UserID, true
}

// Consume atomically validates and removes the challenge token.
func (s *ChallengeStore) Consume(token string) (userID string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, exists := s.challenges[token]
	if !exists || time.Now().After(ch.ExpiresAt) {
		delete(s.challenges, token)
		return "", false
	}
	delete(s.challenges, token)
	return ch.UserID, true
}

func (s *ChallengeStore) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for token, ch := range s.challenges {
		if now.After(ch.ExpiresAt) {
			delete(s.challenges, token)
		}
	}
}

// RevokeForUser removes all challenge tokens belonging to the given user.
func (s *ChallengeStore) RevokeForUser(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, ch := range s.challenges {
		if ch.UserID == userID {
			delete(s.challenges, token)
		}
	}
}
