package resources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/fileproto"
	"github.com/labtether/labtether/internal/persistence"
)

type testFileTransferStore struct {
	mu        sync.Mutex
	transfers map[string]*persistence.FileTransfer
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

func (s *testTransferFileConnectionStore) DeleteFileConnection(_ context.Context, id string) error {
	if _, ok := s.connections[id]; !ok {
		return persistence.ErrNotFound
	}
	delete(s.connections, id)
	return nil
}

func decodeFileTransferTestJSONBody(_ http.ResponseWriter, r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func TestHandleGetFileTransferHidesOtherActors(t *testing.T) {
	deps := &Deps{
		FileTransferStore: testFileTransferStoreWithTransfer("ftx_1", "actor-a", "pending"),
		PrincipalActorID:  apiv2.PrincipalActorID,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/file-transfers/ftx_1", nil)
	req = req.WithContext(apiv2.ContextWithPrincipal(req.Context(), "actor-b", "operator"))
	rec := httptest.NewRecorder()

	deps.HandleFileTransfers(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleCancelFileTransferHidesOtherActors(t *testing.T) {
	deps := &Deps{
		FileTransferStore: testFileTransferStoreWithTransfer("ftx_2", "actor-a", "pending"),
		PrincipalActorID:  apiv2.PrincipalActorID,
		ActiveTransfers:   &sync.Map{},
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/file-transfers/ftx_2", nil)
	req = req.WithContext(apiv2.ContextWithPrincipal(req.Context(), "actor-b", "operator"))
	rec := httptest.NewRecorder()

	deps.HandleFileTransfers(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleGetFileTransferReturnsOwnerTransfer(t *testing.T) {
	deps := &Deps{
		FileTransferStore: testFileTransferStoreWithTransfer("ftx_3", "actor-a", "completed"),
		PrincipalActorID:  apiv2.PrincipalActorID,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/file-transfers/ftx_3", nil)
	req = req.WithContext(apiv2.ContextWithPrincipal(req.Context(), "actor-a", "operator"))
	rec := httptest.NewRecorder()

	deps.HandleFileTransfers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Transfer persistence.FileTransfer `json:"transfer"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Transfer.ID != "ftx_3" {
		t.Fatalf("expected transfer ftx_3, got %q", resp.Transfer.ID)
	}
}

func TestHandleStartFileTransferRejectsOtherActorConnectionBeforeCreate(t *testing.T) {
	pool := fileproto.NewPool()
	defer pool.Close()

	transferStore := newTestFileTransferStore()
	deps := &Deps{
		FileProtoPool: pool,
		FileConnectionStore: newTestTransferFileConnectionStore(
			&persistence.FileConnection{ID: "source-owned-by-a", ActorID: "actor-a"},
			&persistence.FileConnection{ID: "dest-owned-by-b", ActorID: "actor-b"},
		),
		FileTransferStore: transferStore,
		PrincipalActorID:  apiv2.PrincipalActorID,
		ActiveTransfers:   &sync.Map{},
		DecodeJSONBody:    decodeFileTransferTestJSONBody,
	}

	body := strings.NewReader(`{
		"source_type":"connection",
		"source_id":"source-owned-by-a",
		"source_path":"/data/source.txt",
		"dest_type":"connection",
		"dest_id":"dest-owned-by-b",
		"dest_path":"/data/source.txt"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/file-transfers", body)
	req = req.WithContext(apiv2.ContextWithPrincipal(req.Context(), "actor-b", "operator"))
	rec := httptest.NewRecorder()

	deps.HandleFileTransfers(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := transferStore.Count(); got != 0 {
		t.Fatalf("expected no transfer records to be created, got %d", got)
	}
}

func testFileTransferStoreWithTransfer(id, actorID, status string) *testFileTransferStore {
	return newTestFileTransferStore(&persistence.FileTransfer{
		ID:      id,
		ActorID: actorID,
		Status:  status,
	})
}
