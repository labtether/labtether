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
}

func NewChallengeStore() *ChallengeStore {
	return &ChallengeStore{challenges: make(map[string]challenge)}
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
