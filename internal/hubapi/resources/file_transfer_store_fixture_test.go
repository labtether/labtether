package resources

import (
	"context"
	"github.com/labtether/labtether/internal/persistence"
	"sort"
	"strings"
	"sync"
)

type testFileTransferStore struct {
	mu             sync.Mutex
	transfers      map[string]*persistence.FileTransfer
	listErr        error
	lastListActor  string
	lastListStatus string
	lastListLimit  int
	lastListOffset int
	listCalls      int
}

func newTestFileTransferStore(transfers ...*persistence.FileTransfer) *testFileTransferStore {
	store := &testFileTransferStore{transfers: make(map[string]*persistence.FileTransfer, len(transfers))}
	for _, transfer := range transfers {
		cloned := *transfer
		store.transfers[transfer.ID] = &cloned
	}
	return store
}

func (s *testFileTransferStore) GetFileTransfer(_ context.Context, id string) (*persistence.FileTransfer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	transfer, ok := s.transfers[id]
	if !ok {
		return nil, persistence.ErrNotFound
	}
	cloned := *transfer
	return &cloned, nil
}

func (s *testFileTransferStore) CreateFileTransfer(_ context.Context, ft *persistence.FileTransfer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := *ft
	s.transfers[ft.ID] = &cloned
	return nil
}

func (s *testFileTransferStore) UpdateFileTransfer(_ context.Context, ft *persistence.FileTransfer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.transfers[ft.ID]; !ok {
		return persistence.ErrNotFound
	}
	cloned := *ft
	s.transfers[ft.ID] = &cloned
	return nil
}

func (s *testFileTransferStore) ListFileTransfers(_ context.Context, actorID, status string, limit, offset int) ([]persistence.FileTransfer, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastListActor = actorID
	s.lastListStatus = status
	s.lastListLimit = limit
	s.lastListOffset = offset
	s.listCalls++
	if s.listErr != nil {
		return nil, 0, s.listErr
	}

	filtered := make([]persistence.FileTransfer, 0, len(s.transfers))
	for _, transfer := range s.transfers {
		if transfer.ActorID != actorID || (status != "" && transfer.Status != status) {
			continue
		}
		filtered = append(filtered, *transfer)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID > filtered[j].ID })
	total := len(filtered)
	if offset >= total {
		return []persistence.FileTransfer{}, total, nil
	}
	end := min(offset+limit, total)
	return append([]persistence.FileTransfer(nil), filtered[offset:end]...), total, nil
}

func (s *testFileTransferStore) ListActiveFileTransfers(context.Context) ([]persistence.FileTransfer, error) {
	return nil, nil
}

func (s *testFileTransferStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.transfers)
}

type testTransferFileConnectionStore struct {
	connections map[string]*persistence.FileConnection
	pinErr      error
}

func newTestTransferFileConnectionStore(connections ...*persistence.FileConnection) *testTransferFileConnectionStore {
	store := &testTransferFileConnectionStore{connections: make(map[string]*persistence.FileConnection, len(connections))}
	for _, connection := range connections {
		cloned := *connection
		store.connections[connection.ID] = &cloned
	}
	return store
}

func (s *testTransferFileConnectionStore) ListFileConnections(context.Context) ([]persistence.FileConnection, error) {
	out := make([]persistence.FileConnection, 0, len(s.connections))
	for _, connection := range s.connections {
		out = append(out, *connection)
	}
	return out, nil
}

func (s *testTransferFileConnectionStore) GetFileConnection(_ context.Context, id string) (*persistence.FileConnection, error) {
	connection, ok := s.connections[id]
	if !ok {
		return nil, persistence.ErrNotFound
	}
	cloned := *connection
	return &cloned, nil
}

func (s *testTransferFileConnectionStore) CreateFileConnection(_ context.Context, fc *persistence.FileConnection) error {
	cloned := *fc
	s.connections[fc.ID] = &cloned
	return nil
}

func (s *testTransferFileConnectionStore) UpdateFileConnection(_ context.Context, fc *persistence.FileConnection) error {
	if _, ok := s.connections[fc.ID]; !ok {
		return persistence.ErrNotFound
	}
	cloned := *fc
	s.connections[fc.ID] = &cloned
	return nil
}

func (s *testTransferFileConnectionStore) PinSFTPHostKey(_ context.Context, connectionID, expectedHost string, expectedPort int, presentedKey string) error {
	if s.pinErr != nil {
		return s.pinErr
	}
	fc, ok := s.connections[connectionID]
	if !ok {
		return persistence.ErrNotFound
	}
	actualPort := 22
	if fc.Port != nil {
		actualPort = *fc.Port
	}
	if fc.Protocol != "sftp" || fc.Host != expectedHost || actualPort != expectedPort {
		return persistence.ErrFileConnectionChanged
	}
	if existing, pinned := fc.ExtraConfig["host_key"]; pinned {
		if existingKey, ok := existing.(string); !ok || strings.TrimSpace(existingKey) != strings.TrimSpace(presentedKey) {
			return persistence.ErrSFTPHostKeyMismatch
		}
		return nil
	}
	extraConfig := make(map[string]any, len(fc.ExtraConfig)+1)
	for key, value := range fc.ExtraConfig {
		extraConfig[key] = value
	}
	extraConfig["host_key"] = strings.TrimSpace(presentedKey)
	fc.ExtraConfig = extraConfig
	return nil
}

func (s *testTransferFileConnectionStore) DeleteFileConnection(_ context.Context, id string) error {
	if _, ok := s.connections[id]; !ok {
		return persistence.ErrNotFound
	}
	delete(s.connections, id)
	return nil
}

func testFileTransferStoreWithTransfer(id, actorID, status string) *testFileTransferStore {
	return newTestFileTransferStore(&persistence.FileTransfer{
		ID:      id,
		ActorID: actorID,
		Status:  status,
	})
}
