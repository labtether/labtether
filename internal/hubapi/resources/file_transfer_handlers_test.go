package resources

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/fileproto"
	"github.com/labtether/labtether/internal/persistence"
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

func TestHandleStartAgentTransferEnforcesAssetAllowlistBeforeConnectivity(t *testing.T) {
	store := newTestFileTransferStore()
	deps := &Deps{
		AgentMgr:          agentmgr.NewManager(),
		FileBridges:       &sync.Map{},
		FileTransferStore: store,
		PrincipalActorID:  apiv2.PrincipalActorID,
		DecodeJSONBody:    decodeFileTransferTestJSONBody,
	}
	body := strings.NewReader(`{
		"source_type":"agent","source_id":"secret-agent","source_path":"/source.bin",
		"dest_type":"agent","dest_id":"allowed-agent","dest_path":"/dest.bin"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/file-transfers", body)
	ctx := apiv2.ContextWithPrincipal(req.Context(), "actor-a", "operator")
	ctx = apiv2.ContextWithScopes(ctx, []string{"files:read", "files:write"})
	ctx = apiv2.ContextWithAllowedAssets(ctx, []string{"allowed-agent"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	deps.HandleFileTransfers(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if store.Count() != 0 {
		t.Fatal("asset-denied transfer created a persistence record")
	}
}

func TestRemoteFileBaseSupportsAgentPlatforms(t *testing.T) {
	for input, want := range map[string]string{
		"/var/log/system.log":                 "system.log",
		`C:\Users\Michael\Desktop\report.txt`: "report.txt",
		"~/notes.txt":                         "notes.txt",
	} {
		if got := remoteFileBase(input); got != want {
			t.Fatalf("remoteFileBase(%q)=%q want %q", input, got, want)
		}
	}
}

func TestFileTransferBoundedReaderRejectsExtraByteWithoutForwardingIt(t *testing.T) {
	reader := &fileTransferBoundedReader{reader: strings.NewReader("abcd"), remaining: 3}
	payload, err := io.ReadAll(reader)
	if !errors.Is(err, fileproto.ErrTransferTooLarge) {
		t.Fatalf("error=%v, want ErrTransferTooLarge", err)
	}
	if string(payload) != "abc" {
		t.Fatalf("payload=%q, want only bounded bytes", payload)
	}
}

func TestRunAgentBackedFileTransferAgentToAgent(t *testing.T) {
	sourceServer, sourceClient, sourceCleanup := createWoLWebSocketPair(t)
	defer sourceCleanup()
	destServer, destClient, destCleanup := createWoLWebSocketPair(t)
	defer destCleanup()

	manager := agentmgr.NewManager()
	sourceConn := agentmgr.NewAgentConn(sourceServer, "source-agent", "linux")
	destConn := agentmgr.NewAgentConn(destServer, "dest-agent", "linux")
	manager.Register(sourceConn)
	manager.Register(destConn)
	defer manager.Unregister("source-agent")
	defer manager.Unregister("dest-agent")

	transferStore := newTestFileTransferStore(&persistence.FileTransfer{
		ID:         "ftx-agent-agent",
		ActorID:    "actor-a",
		SourceType: "agent",
		SourceID:   "source-agent",
		SourcePath: "/source.bin",
		DestType:   "agent",
		DestID:     "dest-agent",
		DestPath:   "/dest.bin",
		Status:     "pending",
	})
	deps := &Deps{
		AgentMgr:          manager,
		FileBridges:       &sync.Map{},
		FileTransferStore: transferStore,
	}

	sourcePayload := bytes.Repeat([]byte("labtether-agent-transfer-"), 7000)
	responderErrors := make(chan error, 2)
	go func() {
		var request agentmgr.Message
		if err := sourceClient.ReadJSON(&request); err != nil {
			responderErrors <- err
			return
		}
		var read agentmgr.FileReadData
		if err := json.Unmarshal(request.Data, &read); err != nil {
			responderErrors <- err
			return
		}
		cut := len(sourcePayload) / 2
		for _, part := range []struct {
			payload []byte
			offset  int64
			done    bool
		}{
			{payload: sourcePayload[:cut], offset: 0},
			{payload: sourcePayload[cut:], offset: int64(cut), done: true},
		} {
			data, err := json.Marshal(agentmgr.FileDataPayload{
				RequestID: read.RequestID,
				Data:      base64.StdEncoding.EncodeToString(part.payload),
				Offset:    part.offset,
				Done:      part.done,
			})
			if err != nil {
				responderErrors <- err
				return
			}
			deps.ProcessAgentFileData(sourceConn, agentmgr.Message{Type: agentmgr.MsgFileData, ID: read.RequestID, Data: data})
		}
		responderErrors <- nil
	}()

	destinationPayload := make([]byte, 0, len(sourcePayload))
	go func() {
		var requestID string
		for {
			var request agentmgr.Message
			if err := destClient.ReadJSON(&request); err != nil {
				responderErrors <- err
				return
			}
			var write agentmgr.FileWriteData
			if err := json.Unmarshal(request.Data, &write); err != nil {
				responderErrors <- err
				return
			}
			requestID = write.RequestID
			payload, err := base64.StdEncoding.DecodeString(write.Data)
			if err != nil {
				responderErrors <- err
				return
			}
			destinationPayload = append(destinationPayload, payload...)
			if !write.Done {
				continue
			}
			data, err := json.Marshal(agentmgr.FileWrittenData{
				RequestID:    requestID,
				BytesWritten: int64(len(destinationPayload)),
			})
			if err != nil {
				responderErrors <- err
				return
			}
			deps.ProcessAgentFileWritten(destConn, agentmgr.Message{Type: agentmgr.MsgFileWritten, ID: requestID, Data: data})
			responderErrors <- nil
			return
		}
	}()

	markFailed := func(message string) { t.Fatalf("unexpected transfer failure: %s", message) }
	deps.runAgentBackedFileTransfer(context.Background(), "ftx-agent-agent", fileTransferStartRequest{
		SourceType: "agent", SourceID: "source-agent", SourcePath: "/source.bin",
		DestType: "agent", DestID: "dest-agent", DestPath: "/dest.bin",
	}, "actor-a", markFailed)

	for range 2 {
		if err := <-responderErrors; err != nil {
			t.Fatalf("agent responder: %v", err)
		}
	}
	if !bytes.Equal(destinationPayload, sourcePayload) {
		t.Fatalf("destination bytes=%d want=%d", len(destinationPayload), len(sourcePayload))
	}
	transfer, err := transferStore.GetFileTransfer(context.Background(), "ftx-agent-agent")
	if err != nil {
		t.Fatal(err)
	}
	if transfer.Status != "completed" || transfer.BytesTransferred != int64(len(sourcePayload)) || transfer.Error != nil || transfer.FileSize == nil || *transfer.FileSize != int64(len(sourcePayload)) {
		t.Fatalf("transfer=%+v", transfer)
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

func testFileTransferStoreWithTransfer(id, actorID, status string) *testFileTransferStore {
	return newTestFileTransferStore(&persistence.FileTransfer{
		ID:      id,
		ActorID: actorID,
		Status:  status,
	})
}
