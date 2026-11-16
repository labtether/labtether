package main

import (
	"context"
	"encoding/json"
	"github.com/labtether/labtether/internal/persistence"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/apiv2"
)

// ─── POST/GET /api/v2/file-transfers ──────────────────────────────────────

type v2FileTransferListStore struct {
	transfers []persistence.FileTransfer
	actorID   string
	status    string
	limit     int
	offset    int
}

func (s *v2FileTransferListStore) GetFileTransfer(_ context.Context, id string) (*persistence.FileTransfer, error) {
	for i := range s.transfers {
		if s.transfers[i].ID == id {
			transfer := s.transfers[i]
			return &transfer, nil
		}
	}
	return nil, persistence.ErrNotFound
}

func (s *v2FileTransferListStore) CreateFileTransfer(_ context.Context, transfer *persistence.FileTransfer) error {
	s.transfers = append(s.transfers, *transfer)
	return nil
}

func (s *v2FileTransferListStore) UpdateFileTransfer(_ context.Context, transfer *persistence.FileTransfer) error {
	for i := range s.transfers {
		if s.transfers[i].ID == transfer.ID {
			s.transfers[i] = *transfer
			return nil
		}
	}
	return persistence.ErrNotFound
}

func (s *v2FileTransferListStore) ListFileTransfers(_ context.Context, actorID, status string, limit, offset int) ([]persistence.FileTransfer, int, error) {
	s.actorID = actorID
	s.status = status
	s.limit = limit
	s.offset = offset
	filtered := make([]persistence.FileTransfer, 0, len(s.transfers))
	for _, transfer := range s.transfers {
		if transfer.ActorID == actorID && (status == "" || transfer.Status == status) {
			filtered = append(filtered, transfer)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID > filtered[j].ID })
	total := len(filtered)
	if offset >= total {
		return []persistence.FileTransfer{}, total, nil
	}
	return append([]persistence.FileTransfer(nil), filtered[offset:min(offset+limit, total)]...), total, nil
}

func (s *v2FileTransferListStore) ListActiveFileTransfers(context.Context) ([]persistence.FileTransfer, error) {
	return []persistence.FileTransfer{}, nil
}

func TestHandleV2FileTransfers_GetScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/file-transfers", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = apiv2.ContextWithScopes(ctx, []string{"assets:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2FileTransfers(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2FileTransfers_GetDelegatesActorScopedFilteredPage(t *testing.T) {
	s := newTestAPIServer(t)
	store := &v2FileTransferListStore{transfers: []persistence.FileTransfer{
		{ID: "ftx_500", ActorID: "actor-b", Status: "completed"},
		{ID: "ftx_400", ActorID: "actor-a", Status: "pending"},
		{ID: "ftx_300", ActorID: "actor-a", Status: "completed"},
		{ID: "ftx_100", ActorID: "actor-a", Status: "completed"},
	}}
	s.fileTransferStore = store
	req := httptest.NewRequest(http.MethodGet, "/api/v2/file-transfers?status=completed&limit=1&offset=1", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "actor-a", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2FileTransfers(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		RequestID string `json:"request_id"`
		Data      struct {
			Transfers []persistence.FileTransfer `json:"transfers"`
			Total     int                        `json:"total"`
			Limit     int                        `json:"limit"`
			Offset    int                        `json:"offset"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.RequestID == "" {
		t.Fatal("v2 response is missing request_id")
	}
	if response.Data.Total != 2 || response.Data.Limit != 1 || response.Data.Offset != 1 {
		t.Fatalf("pagination=%+v", response.Data)
	}
	if len(response.Data.Transfers) != 1 || response.Data.Transfers[0].ID != "ftx_100" {
		t.Fatalf("transfers=%+v, want second matching actor-a record", response.Data.Transfers)
	}
	if strings.Contains(rec.Body.String(), "ftx_500") || strings.Contains(rec.Body.String(), "actor-a") || strings.Contains(rec.Body.String(), "actor-b") {
		t.Fatalf("v2 response disclosed actor data: %s", rec.Body.String())
	}
	if store.actorID != "actor-a" || store.status != "completed" || store.limit != 1 || store.offset != 1 {
		t.Fatalf("delegated query actor=%q status=%q limit=%d offset=%d", store.actorID, store.status, store.limit, store.offset)
	}
}

func TestHandleV2FileTransfers_GetRejectsOutOfBoundsPagination(t *testing.T) {
	s := newTestAPIServer(t)
	store := &v2FileTransferListStore{}
	s.fileTransferStore = store
	req := httptest.NewRequest(http.MethodGet, "/api/v2/file-transfers?limit=101", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "actor-a", "admin"))
	rec := httptest.NewRecorder()

	s.handleV2FileTransfers(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.actorID != "" {
		t.Fatalf("invalid pagination reached persistence for actor %q", store.actorID)
	}
}

func TestHandleV2FileTransfers_PostScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	body := `{"source_type":"connection","source_id":"x","source_path":"/x","dest_type":"connection","dest_id":"y","dest_path":"/y"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/file-transfers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = apiv2.ContextWithScopes(ctx, []string{"files:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2FileTransfers(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2FileTransfers_MethodNotAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v2/file-transfers", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2FileTransfers(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

// ─── GET /api/v2/file-transfers/{id} ──────────────────────────────────────

func TestHandleV2FileTransferActions_MissingID(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/file-transfers/", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2FileTransferActions(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing ID, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2FileTransferActions_GetScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/file-transfers/ft_abc", nil)
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = apiv2.ContextWithScopes(ctx, []string{"assets:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2FileTransferActions(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2FileTransferActions_MethodNotAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v2/file-transfers/ft_abc", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2FileTransferActions(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

// ─── POST /api/v2/bulk/file-push ──────────────────────────────────────────

func TestHandleV2BulkFilePush_ScopeDenied(t *testing.T) {
	s := newTestAPIServer(t)
	body := `{"source_connection_id":"c1","source_path":"/x","targets":[]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/file-push", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = apiv2.ContextWithScopes(ctx, []string{"files:read"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	s.handleV2BulkFilePush(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestHandleV2BulkFilePush_MethodNotAllowed(t *testing.T) {
	s := newTestAPIServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v2/bulk/file-push", nil)
	req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
	rec := httptest.NewRecorder()
	s.handleV2BulkFilePush(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleV2BulkFilePush_RejectsTooManyTargets(t *testing.T) {
	s := newTestAPIServer(t)
	targets := make([]map[string]string, 0, maxBulkFilePushTargets+1)
	for i := 0; i < maxBulkFilePushTargets+1; i++ {
		targets = append(targets, map[string]string{
			"dest_connection_id": "conn",
			"dest_path":          "/tmp/out",
		})
	}
	body, err := json.Marshal(map[string]any{
		"source_connection_id": "c1",
		"source_path":          "/x",
		"targets":              targets,
	})
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/file-push", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	ctx := contextWithPrincipal(req.Context(), "apikey:k1", "operator")
	ctx = apiv2.ContextWithScopes(ctx, []string{"files:write"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	s.handleV2BulkFilePush(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleV2BulkFilePush_ValidationErrors(t *testing.T) {
	s := newTestAPIServer(t)

	cases := []struct {
		name string
		body string
		code int
	}{
		{
			name: "missing source_connection_id",
			body: `{"source_path":"/x","targets":[{"dest_connection_id":"c2","dest_path":"/y"}]}`,
			code: http.StatusBadRequest,
		},
		{
			name: "missing source_path",
			body: `{"source_connection_id":"c1","targets":[{"dest_connection_id":"c2","dest_path":"/y"}]}`,
			code: http.StatusBadRequest,
		},
		{
			name: "empty targets",
			body: `{"source_connection_id":"c1","source_path":"/x","targets":[]}`,
			code: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v2/bulk/file-push", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(contextWithPrincipal(req.Context(), "admin", "admin"))
			rec := httptest.NewRecorder()
			s.handleV2BulkFilePush(rec, req)
			if rec.Code != tc.code {
				t.Fatalf("expected %d, got %d: %s", tc.code, rec.Code, rec.Body.String())
			}
		})
	}
}
