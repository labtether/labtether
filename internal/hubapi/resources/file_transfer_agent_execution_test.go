package resources

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/fileproto"
	"github.com/labtether/labtether/internal/persistence"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

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
