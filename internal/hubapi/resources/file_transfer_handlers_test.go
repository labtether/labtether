package resources

import (
	"encoding/json"
	"errors"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/fileproto"
	"github.com/labtether/labtether/internal/persistence"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func decodeFileTransferTestJSONBody(_ http.ResponseWriter, r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func TestHandleListFileTransfersIsActorScopedAndNewestFirst(t *testing.T) {
	store := newTestFileTransferStore(
		&persistence.FileTransfer{ID: "ftx_100", ActorID: "actor-a", Status: "completed"},
		&persistence.FileTransfer{ID: "ftx_300", ActorID: "actor-b", Status: "completed"},
		&persistence.FileTransfer{ID: "ftx_200", ActorID: "actor-a", Status: "pending"},
	)
	deps := &Deps{
		FileTransferStore: store,
		PrincipalActorID:  apiv2.PrincipalActorID,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/file-transfers", nil)
	req = req.WithContext(apiv2.ContextWithPrincipal(req.Context(), "actor-a", "operator"))
	rec := httptest.NewRecorder()

	deps.HandleFileTransfers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Transfers []persistence.FileTransfer `json:"transfers"`
		Total     int                        `json:"total"`
		Limit     int                        `json:"limit"`
		Offset    int                        `json:"offset"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Total != 2 || response.Limit != persistence.FileTransferListDefaultLimit || response.Offset != 0 {
		t.Fatalf("pagination=%+v", response)
	}
	if len(response.Transfers) != 2 || response.Transfers[0].ID != "ftx_200" || response.Transfers[1].ID != "ftx_100" {
		t.Fatalf("transfers=%+v, want actor-a records newest-first", response.Transfers)
	}
	if strings.Contains(rec.Body.String(), "ftx_300") || strings.Contains(rec.Body.String(), "actor-a") || strings.Contains(rec.Body.String(), "actor-b") {
		t.Fatalf("response disclosed hidden actor data: %s", rec.Body.String())
	}
	if store.lastListActor != "actor-a" {
		t.Fatalf("store actor=%q, want actor-a", store.lastListActor)
	}
}

func TestHandleListFileTransfersAppliesStatusAndPaginationBeforeDisclosure(t *testing.T) {
	store := newTestFileTransferStore(
		&persistence.FileTransfer{ID: "ftx_400", ActorID: "actor-a", Status: "pending"},
		&persistence.FileTransfer{ID: "ftx_300", ActorID: "actor-a", Status: "completed"},
		&persistence.FileTransfer{ID: "ftx_200", ActorID: "actor-b", Status: "completed"},
		&persistence.FileTransfer{ID: "ftx_100", ActorID: "actor-a", Status: "completed"},
	)
	deps := &Deps{
		FileTransferStore: store,
		PrincipalActorID:  apiv2.PrincipalActorID,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/file-transfers?status=COMPLETED&limit=1&offset=1", nil)
	req = req.WithContext(apiv2.ContextWithPrincipal(req.Context(), "actor-a", "operator"))
	rec := httptest.NewRecorder()

	deps.HandleFileTransfers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Transfers []persistence.FileTransfer `json:"transfers"`
		Total     int                        `json:"total"`
		Limit     int                        `json:"limit"`
		Offset    int                        `json:"offset"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Total != 2 || response.Limit != 1 || response.Offset != 1 {
		t.Fatalf("pagination=%+v", response)
	}
	if len(response.Transfers) != 1 || response.Transfers[0].ID != "ftx_100" {
		t.Fatalf("transfers=%+v, want second actor-a completed transfer", response.Transfers)
	}
	if store.lastListStatus != "completed" || store.lastListLimit != 1 || store.lastListOffset != 1 {
		t.Fatalf("store query status=%q limit=%d offset=%d", store.lastListStatus, store.lastListLimit, store.lastListOffset)
	}
}

func TestHandleListFileTransfersRejectsInvalidFiltersBeforePersistence(t *testing.T) {
	tests := []string{
		"limit=0",
		"limit=101",
		"limit=not-a-number",
		"offset=-1",
		"offset=10001",
		"offset=not-a-number",
		"status=cancelled",
	}
	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			store := newTestFileTransferStore()
			deps := &Deps{
				FileTransferStore: store,
				PrincipalActorID:  apiv2.PrincipalActorID,
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/file-transfers?"+query, nil)
			req = req.WithContext(apiv2.ContextWithPrincipal(req.Context(), "actor-a", "operator"))
			rec := httptest.NewRecorder()

			deps.HandleFileTransfers(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if store.listCalls != 0 {
				t.Fatalf("invalid query reached persistence %d times", store.listCalls)
			}
		})
	}
}

func TestHandleListFileTransfersAcceptsInclusiveMaximumBounds(t *testing.T) {
	store := newTestFileTransferStore()
	deps := &Deps{
		FileTransferStore: store,
		PrincipalActorID:  apiv2.PrincipalActorID,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/file-transfers?limit=100&offset=10000", nil)
	req = req.WithContext(apiv2.ContextWithPrincipal(req.Context(), "actor-a", "operator"))
	rec := httptest.NewRecorder()

	deps.HandleFileTransfers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if store.lastListLimit != persistence.FileTransferListMaxLimit || store.lastListOffset != persistence.FileTransferListMaxOffset {
		t.Fatalf("store limit=%d offset=%d", store.lastListLimit, store.lastListOffset)
	}
}

func TestHandleListFileTransfersReturnsEmptyArrayAndSanitizesStoreErrors(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		deps := &Deps{
			FileTransferStore: newTestFileTransferStore(),
			PrincipalActorID:  apiv2.PrincipalActorID,
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/file-transfers", nil)
		req = req.WithContext(apiv2.ContextWithPrincipal(req.Context(), "actor-a", "operator"))
		rec := httptest.NewRecorder()

		deps.HandleFileTransfers(rec, req)

		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"transfers":[]`) {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("store error", func(t *testing.T) {
		store := newTestFileTransferStore()
		store.listErr = errors.New("private database detail")
		deps := &Deps{
			FileTransferStore: store,
			PrincipalActorID:  apiv2.PrincipalActorID,
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/file-transfers", nil)
		req = req.WithContext(apiv2.ContextWithPrincipal(req.Context(), "actor-a", "operator"))
		rec := httptest.NewRecorder()

		deps.HandleFileTransfers(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "private database detail") {
			t.Fatalf("response disclosed store error: %s", rec.Body.String())
		}
	})
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

func TestHandleStartFileTransferRejectsWhenAdmissionIsFullBeforeCreate(t *testing.T) {
	releases := make([]func(), 0, maxConcurrentFileTransfers)
	for i := 0; i < maxConcurrentFileTransfers; i++ {
		release, ok := fileTransferAdmission.tryAcquire()
		if !ok {
			t.Fatalf("acquire slot %d", i)
		}
		releases = append(releases, release)
	}
	defer func() {
		for _, release := range releases {
			release()
		}
	}()

	pool := fileproto.NewPool()
	defer pool.Close()
	transferStore := newTestFileTransferStore()
	deps := &Deps{
		FileProtoPool: pool,
		FileConnectionStore: newTestTransferFileConnectionStore(
			&persistence.FileConnection{ID: "source-a", ActorID: "actor-a"},
			&persistence.FileConnection{ID: "dest-a", ActorID: "actor-a"},
		),
		FileTransferStore: transferStore,
		PrincipalActorID:  apiv2.PrincipalActorID,
		ActiveTransfers:   &sync.Map{},
		DecodeJSONBody:    decodeFileTransferTestJSONBody,
	}
	body := strings.NewReader(`{
		"source_type":"connection",
		"source_id":"source-a",
		"source_path":"/source.bin",
		"dest_type":"connection",
		"dest_id":"dest-a",
		"dest_path":"/dest.bin"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/file-transfers", body)
	req = req.WithContext(apiv2.ContextWithPrincipal(req.Context(), "actor-a", "operator"))
	rec := httptest.NewRecorder()

	deps.HandleFileTransfers(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After=%q, want 1", got)
	}
	if got := transferStore.Count(); got != 0 {
		t.Fatalf("overload created %d transfer records", got)
	}
}

func TestHandleStartFileTransferRequiresReadAndWriteScopes(t *testing.T) {
	pool := fileproto.NewPool()
	defer pool.Close()
	store := newTestFileTransferStore()
	deps := &Deps{
		FileProtoPool: pool,
		FileConnectionStore: newTestTransferFileConnectionStore(
			&persistence.FileConnection{ID: "source-a", ActorID: "actor-a"},
			&persistence.FileConnection{ID: "dest-a", ActorID: "actor-a"},
		),
		FileTransferStore: store,
		PrincipalActorID:  apiv2.PrincipalActorID,
		DecodeJSONBody:    decodeFileTransferTestJSONBody,
	}
	body := strings.NewReader(`{
		"source_type":"connection","source_id":"source-a","source_path":"/source.bin",
		"dest_type":"connection","dest_id":"dest-a","dest_path":"/dest.bin"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/file-transfers", body)
	ctx := apiv2.ContextWithPrincipal(req.Context(), "actor-a", "operator")
	ctx = apiv2.ContextWithScopes(ctx, []string{"files:write"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	deps.HandleFileTransfers(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if store.Count() != 0 {
		t.Fatal("scope-denied transfer created a persistence record")
	}
}

func TestValidateTransferRequestBoundsEndpointIDsAndPaths(t *testing.T) {
	valid := fileTransferStartRequest{
		SourceType: "agent", SourceID: "source", SourcePath: "/source",
		DestType: "agent", DestID: "dest", DestPath: "/dest",
	}
	for name, mutate := range map[string]func(*fileTransferStartRequest){
		"source id": func(req *fileTransferStartRequest) {
			req.SourceID = strings.Repeat("a", maxFileTransferEndpointIDBytes+1)
		},
		"destination id": func(req *fileTransferStartRequest) {
			req.DestID = strings.Repeat("a", maxFileTransferEndpointIDBytes+1)
		},
		"source path":      func(req *fileTransferStartRequest) { req.SourcePath = strings.Repeat("a", maxFileTransferPathBytes+1) },
		"destination path": func(req *fileTransferStartRequest) { req.DestPath = "bad\x00path" },
	} {
		t.Run(name, func(t *testing.T) {
			req := valid
			mutate(&req)
			if err := validateTransferRequest(req); err == nil {
				t.Fatal("expected bounded validation failure")
			}
		})
	}
}
