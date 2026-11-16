package testutil

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/terminal"
)

// MemoryTerminalBookmarkStore is an in-memory persistence.TerminalBookmarkStore
// for terminal tests (cmd's test server has none). IDs are "bkm-test-<n>".
// cmd/labtether keeps a one-line newMemoryTerminalBookmarkStore wrapper.
type MemoryTerminalBookmarkStore struct {
	mu        sync.RWMutex
	bookmarks map[string]terminal.Bookmark
	nextID    int
}

func NewMemoryTerminalBookmarkStore() *MemoryTerminalBookmarkStore {
	return &MemoryTerminalBookmarkStore{bookmarks: make(map[string]terminal.Bookmark)}
}

func (s *MemoryTerminalBookmarkStore) CreateBookmark(req terminal.CreateBookmarkRequest) (terminal.Bookmark, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	now := time.Now().UTC()
	bookmark := terminal.Bookmark{
		ID:                  "bkm-test-" + strconv.Itoa(s.nextID),
		ActorID:             strings.TrimSpace(req.ActorID),
		Title:               strings.TrimSpace(req.Title),
		AssetID:             strings.TrimSpace(req.AssetID),
		Host:                strings.TrimSpace(req.Host),
		Port:                req.Port,
		Username:            strings.TrimSpace(req.Username),
		CredentialProfileID: strings.TrimSpace(req.CredentialProfileID),
		JumpChainGroupID:    strings.TrimSpace(req.JumpChainGroupID),
		Tags:                append([]string(nil), req.Tags...),
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	s.bookmarks[bookmark.ID] = bookmark
	return bookmark, nil
}

func (s *MemoryTerminalBookmarkStore) GetBookmark(id string) (terminal.Bookmark, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bookmark, ok := s.bookmarks[strings.TrimSpace(id)]
	return bookmark, ok, nil
}

func (s *MemoryTerminalBookmarkStore) ListBookmarks(actorID string) ([]terminal.Bookmark, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]terminal.Bookmark, 0, len(s.bookmarks))
	for _, bookmark := range s.bookmarks {
		if bookmark.ActorID == strings.TrimSpace(actorID) {
			out = append(out, bookmark)
		}
	}
	return out, nil
}

func (s *MemoryTerminalBookmarkStore) UpdateBookmark(id string, req terminal.UpdateBookmarkRequest) (terminal.Bookmark, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	bookmark, ok := s.bookmarks[strings.TrimSpace(id)]
	if !ok {
		return terminal.Bookmark{}, persistence.ErrNotFound
	}
	if req.Title != nil {
		bookmark.Title = strings.TrimSpace(*req.Title)
	}
	if req.AssetID != nil {
		bookmark.AssetID = strings.TrimSpace(*req.AssetID)
	}
	if req.Host != nil {
		bookmark.Host = strings.TrimSpace(*req.Host)
	}
	if req.Port != nil {
		bookmark.Port = req.Port
	}
	if req.Username != nil {
		bookmark.Username = strings.TrimSpace(*req.Username)
	}
	if req.CredentialProfileID != nil {
		bookmark.CredentialProfileID = strings.TrimSpace(*req.CredentialProfileID)
	}
	if req.JumpChainGroupID != nil {
		bookmark.JumpChainGroupID = strings.TrimSpace(*req.JumpChainGroupID)
	}
	if req.Tags != nil {
		bookmark.Tags = append([]string(nil), req.Tags...)
	}
	bookmark.UpdatedAt = time.Now().UTC()
	s.bookmarks[bookmark.ID] = bookmark
	return bookmark, nil
}

func (s *MemoryTerminalBookmarkStore) DeleteBookmark(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.bookmarks[strings.TrimSpace(id)]; !ok {
		return persistence.ErrNotFound
	}
	delete(s.bookmarks, strings.TrimSpace(id))
	return nil
}

func (s *MemoryTerminalBookmarkStore) SetBookmarkHostKey(id, hostKey, target string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	bookmark, ok := s.bookmarks[strings.TrimSpace(id)]
	if !ok {
		return persistence.ErrNotFound
	}
	bookmark.HostKey, bookmark.HostKeyTarget = hostKey, target
	s.bookmarks[bookmark.ID] = bookmark
	return nil
}

func (s *MemoryTerminalBookmarkStore) TouchBookmarkLastUsed(id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	bookmark, ok := s.bookmarks[strings.TrimSpace(id)]
	if !ok {
		return persistence.ErrNotFound
	}
	atCopy := at.UTC()
	bookmark.LastUsedAt = &atCopy
	bookmark.UpdatedAt = atCopy
	s.bookmarks[bookmark.ID] = bookmark
	return nil
}
