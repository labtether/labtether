package terminal

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/idgen"
)

var ErrSessionNotFound = errors.New("session not found")

type Store struct {
	mu                 sync.RWMutex
	sessions           map[string]Session
	commands           map[string][]Command
	persistentSessions map[string]PersistentSession
	scrollbackBuffers  map[string]*RingBuffer // keyed by persistent session ID
}

func NewStore() *Store {
	return &Store{
		sessions:           make(map[string]Session),
		commands:           make(map[string][]Command),
		persistentSessions: make(map[string]PersistentSession),
		scrollbackBuffers:  make(map[string]*RingBuffer),
	}
}

// GetOrCreateScrollbackBuffer returns an existing ring buffer for the given
// persistent session ID, or creates a new one with 20 000 lines capacity.
func (s *Store) GetOrCreateScrollbackBuffer(persistentSessionID string) *RingBuffer {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rb, ok := s.scrollbackBuffers[persistentSessionID]; ok {
		return rb
	}
	rb := NewRingBuffer(20000)
	s.scrollbackBuffers[persistentSessionID] = rb
	return rb
}

// RemoveScrollbackBuffer removes the in-memory ring buffer for a persistent session.
func (s *Store) RemoveScrollbackBuffer(persistentSessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.scrollbackBuffers, persistentSessionID)
}

func (s *Store) CreateSession(req CreateSessionRequest) Session {
	now := time.Now().UTC()
	mode := req.Mode
	if mode == "" {
		mode = "interactive"
	}

	session := Session{
		ID:                  idgen.New("sess"),
		ActorID:             req.ActorID,
		Target:              req.Target,
		Mode:                mode,
		Status:              "active",
		PersistentSessionID: req.PersistentSessionID,
		TmuxSessionName:     req.TmuxSessionName,
		CreatedAt:           now,
		LastActionAt:        now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
	return session
}

func (s *Store) UpdateSession(session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[session.ID]; !ok {
		return ErrSessionNotFound
	}
	s.sessions[session.ID] = session
	return nil
}

func (s *Store) GetSession(id string) (Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[id]
	return session, ok
}

func (s *Store) ListSessions() []Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		out = append(out, session)
	}
	return out
}

func (s *Store) CreateOrUpdatePersistentSession(req CreatePersistentSessionRequest) PersistentSession {
	now := time.Now().UTC()
	target := req.Target
	title := req.Title

	s.mu.Lock()
	defer s.mu.Unlock()

	for id, existing := range s.persistentSessions {
		if existing.ActorID == req.ActorID && existing.Target == target {
			existing.Title = title
			if existing.Title == "" {
				existing.Title = target
			}
			if req.BookmarkID != "" {
				existing.BookmarkID = req.BookmarkID
			}
			existing.UpdatedAt = now
			s.persistentSessions[id] = existing
			return existing
		}
	}

	persistent := PersistentSession{
		ID:         idgen.New("pts"),
		ActorID:    req.ActorID,
		Target:     target,
		Title:      title,
		Status:     "detached",
		BookmarkID: req.BookmarkID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if persistent.Title == "" {
		persistent.Title = target
	}
	persistent.TmuxSessionName = buildPersistentTmuxSessionName(persistent.ID)
	s.persistentSessions[persistent.ID] = persistent
	return persistent
}

func (s *Store) GetPersistentSession(id string) (PersistentSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	persistent, ok := s.persistentSessions[id]
	return persistent, ok
}

func (s *Store) ListPersistentSessions() []PersistentSession {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]PersistentSession, 0, len(s.persistentSessions))
	for _, persistent := range s.persistentSessions {
		out = append(out, persistent)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (s *Store) UpdatePersistentSession(id string, req UpdatePersistentSessionRequest) (PersistentSession, error) {
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	persistent, ok := s.persistentSessions[id]
	if !ok {
		return PersistentSession{}, ErrSessionNotFound
	}
	if req.Title != nil {
		persistent.Title = *req.Title
		if persistent.Title == "" {
			persistent.Title = persistent.Target
		}
	}
	if req.Status != nil {
		persistent.Status = *req.Status
	}
	persistent.UpdatedAt = now
	s.persistentSessions[id] = persistent
	return persistent, nil
}

func (s *Store) MarkPersistentSessionAttached(id string, attachedAt time.Time) (PersistentSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	persistent, ok := s.persistentSessions[id]
	if !ok {
		return PersistentSession{}, ErrSessionNotFound
	}
	persistent.Status = "attached"
	persistent.LastAttachedAt = &attachedAt
	persistent.UpdatedAt = attachedAt
	s.persistentSessions[id] = persistent
	return persistent, nil
}

func (s *Store) MarkPersistentSessionDetached(id string, detachedAt time.Time) (PersistentSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	persistent, ok := s.persistentSessions[id]
	if !ok {
		return PersistentSession{}, ErrSessionNotFound
	}
	persistent.Status = "detached"
	persistent.LastDetachedAt = &detachedAt
	persistent.UpdatedAt = detachedAt
	s.persistentSessions[id] = persistent
	return persistent, nil
}

func (s *Store) DeletePersistentSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.persistentSessions[id]; !ok {
		return ErrSessionNotFound
	}
	delete(s.persistentSessions, id)
	return nil
}

func (s *Store) DeleteTerminalSession(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return ErrSessionNotFound
	}
	delete(s.sessions, sessionID)
	delete(s.commands, sessionID)
	return nil
}

func (s *Store) AddCommand(sessionID string, req CreateCommandRequest, target, mode string) (Command, error) {
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return Command{}, ErrSessionNotFound
	}

	cmd := Command{
		ID:        idgen.New("cmd"),
		SessionID: sessionID,
		ActorID:   req.ActorID,
		Target:    target,
		Body:      req.Command,
		Mode:      mode,
		Status:    "queued",
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.commands[sessionID] = append(s.commands[sessionID], cmd)
	session.LastActionAt = now
	s.sessions[sessionID] = session

	return cmd, nil
}

func (s *Store) UpdateCommandResult(sessionID, commandID, status, output string) error {
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	list, ok := s.commands[sessionID]
	if !ok {
		return ErrSessionNotFound
	}

	for idx := range list {
		if list[idx].ID == commandID {
			list[idx].Status = status
			list[idx].Output = output
			list[idx].UpdatedAt = now
			s.commands[sessionID] = list
			return nil
		}
	}
	return errors.New("command not found")
}

func (s *Store) ListCommands(sessionID string) ([]Command, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return nil, ErrSessionNotFound
	}

	list := s.commands[sessionID]
	out := make([]Command, len(list))
	copy(out, list)
	return out, nil
}

func buildPersistentTmuxSessionName(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return "lt-shell"
	}
	name := "lt-" + trimmed
	if len(name) > 24 {
		name = name[:24]
	}
	return name
}
