package resources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
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

func testFileTransferStoreWithTransfer(id, actorID, status string) *testFileTransferStore {
	return newTestFileTransferStore(&persistence.FileTransfer{
		ID:      id,
		ActorID: actorID,
		Status:  status,
	})
}
