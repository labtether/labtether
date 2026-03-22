package agentcore

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentcore/files"
	"github.com/labtether/labtether/internal/agentmgr"
)

func TestTerminalManagerProbeReportsTmuxAvailability(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	tm := newTerminalManager()
	tm.HandleTerminalProbe(transport)

	msg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgTerminalProbed, 2*time.Second)
	var probe agentmgr.TerminalProbeResponse
	if err := json.Unmarshal(msg.Data, &probe); err != nil {
		t.Fatalf("decode terminal probe payload: %v", err)
	}

	tmuxPath, err := exec.LookPath("tmux")
	wantHasTmux := err == nil && tmuxPath != ""
	if probe.HasTmux != wantHasTmux {
		t.Fatalf("expected has_tmux=%v, got %v", wantHasTmux, probe.HasTmux)
	}
	if wantHasTmux && probe.TmuxPath != tmuxPath {
		t.Fatalf("expected tmux path %q, got %q", tmuxPath, probe.TmuxPath)
	}
}

func TestTerminalManagerTmuxKillEndsSavedSession(t *testing.T) {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil || strings.TrimSpace(tmuxPath) == "" {
		t.Skip("tmux not available")
	}

	sessionName := "labtether-test-kill-" + strings.ReplaceAll(t.Name(), "/", "-")
	createCmd := exec.Command(tmuxPath, "new-session", "-d", "-s", sessionName)
	if output, err := createCmd.CombinedOutput(); err != nil {
		t.Fatalf("create tmux session: %v output=%s", err, strings.TrimSpace(string(output)))
	}
	defer func() {
		_ = exec.Command(tmuxPath, "kill-session", "-t", sessionName).Run()
	}()

	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	tm := newTerminalManager()
	raw, err := json.Marshal(agentmgr.TerminalTmuxKillData{
		JobID:       "job-kill",
		SessionID:   "sess-kill",
		CommandID:   "persistent.tmux.kill",
		TmuxSession: sessionName,
		Timeout:     5,
	})
	if err != nil {
		t.Fatalf("marshal terminal tmux kill request: %v", err)
	}

	tm.HandleTerminalTmuxKill(transport, agentmgr.Message{Data: raw})

	msg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgCommandResult, 5*time.Second)
	var result agentmgr.CommandResultData
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		t.Fatalf("decode tmux kill result: %v", err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("expected succeeded tmux kill result, got %+v", result)
	}

	checkCmd := exec.Command(tmuxPath, "has-session", "-t", sessionName)
	if err := checkCmd.Run(); err == nil {
		t.Fatalf("expected tmux session %q to be gone after kill", sessionName)
	}
}

func TestTerminalManagerStartStreamsOutputAndCleansUp(t *testing.T) {
	shellPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}

	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	tm := newTerminalManager()
	defer tm.CloseAll()

	startRaw, err := json.Marshal(agentmgr.TerminalStartData{
		SessionID: "sess-1",
		Shell:     shellPath,
		Cols:      80,
		Rows:      24,
	})
	if err != nil {
		t.Fatalf("marshal terminal start: %v", err)
	}

	tm.HandleTerminalStart(transport, agentmgr.Message{Data: startRaw})

	startedMsg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgTerminalStarted, 5*time.Second)
	var started agentmgr.TerminalStartedData
	if err := json.Unmarshal(startedMsg.Data, &started); err != nil {
		t.Fatalf("decode terminal started payload: %v", err)
	}
	if started.SessionID != "sess-1" {
		t.Fatalf("expected session_id sess-1, got %q", started.SessionID)
	}

	inputRaw, err := json.Marshal(agentmgr.TerminalDataPayload{
		SessionID: "sess-1",
		Data:      base64.StdEncoding.EncodeToString([]byte("printf 'terminal-ok\\n'; exit\n")),
	})
	if err != nil {
		t.Fatalf("marshal terminal input: %v", err)
	}
	tm.HandleTerminalData(agentmgr.Message{Data: inputRaw})

	var output strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case msg, ok := <-messages:
			if !ok {
				t.Fatal("transport closed before terminal session finished")
			}
			switch msg.Type {
			case agentmgr.MsgTerminalData:
				var payload agentmgr.TerminalDataPayload
				if err := json.Unmarshal(msg.Data, &payload); err != nil {
					t.Fatalf("decode terminal data payload: %v", err)
				}
				decoded, err := base64.StdEncoding.DecodeString(payload.Data)
				if err != nil {
					t.Fatalf("decode terminal data chunk: %v", err)
				}
				output.Write(decoded)
			case agentmgr.MsgTerminalClosed:
				var closed agentmgr.TerminalCloseData
				if err := json.Unmarshal(msg.Data, &closed); err != nil {
					t.Fatalf("decode terminal closed payload: %v", err)
				}
				if closed.SessionID != "sess-1" {
					t.Fatalf("expected closed session_id sess-1, got %q", closed.SessionID)
				}
				if !strings.Contains(closed.Reason, "shell exited") {
					t.Fatalf("expected shell-exit reason, got %q", closed.Reason)
				}
				if !strings.Contains(output.String(), "terminal-ok") {
					t.Fatalf("expected terminal output to contain command result, got %q", output.String())
				}

				tm.Mu.Lock()
				_, exists := tm.Sessions["sess-1"]
				tm.Mu.Unlock()
				if exists {
					t.Fatalf("expected terminal session cleanup after close")
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for terminal output and close")
		}
	}
}

func TestTerminalManagerLargeOutputEmitsMultipleDataFrames(t *testing.T) {
	shellPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}

	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	tm := newTerminalManager()
	defer tm.CloseAll()

	startRaw, err := json.Marshal(agentmgr.TerminalStartData{
		SessionID: "sess-large-output",
		Shell:     shellPath,
		Cols:      80,
		Rows:      24,
	})
	if err != nil {
		t.Fatalf("marshal terminal start: %v", err)
	}

	tm.HandleTerminalStart(transport, agentmgr.Message{Data: startRaw})
	_ = waitForCapturedAgentMessage(t, messages, agentmgr.MsgTerminalStarted, 5*time.Second)

	inputRaw, err := json.Marshal(agentmgr.TerminalDataPayload{
		SessionID: "sess-large-output",
		Data: base64.StdEncoding.EncodeToString([]byte(
			"printf 'BEGIN:'; i=0; while [ $i -lt 6000 ]; do printf x; i=$((i+1)); done; printf ':END\\n'; exit\n",
		)),
	})
	if err != nil {
		t.Fatalf("marshal terminal input: %v", err)
	}
	tm.HandleTerminalData(agentmgr.Message{Data: inputRaw})

	var (
		output     strings.Builder
		dataFrames int
	)
	deadline := time.After(10 * time.Second)
	for {
		select {
		case msg, ok := <-messages:
			if !ok {
				t.Fatal("transport closed before terminal session finished")
			}
			switch msg.Type {
			case agentmgr.MsgTerminalData:
				var payload agentmgr.TerminalDataPayload
				if err := json.Unmarshal(msg.Data, &payload); err != nil {
					t.Fatalf("decode terminal data payload: %v", err)
				}
				decoded, err := base64.StdEncoding.DecodeString(payload.Data)
				if err != nil {
					t.Fatalf("decode terminal data chunk: %v", err)
				}
				dataFrames++
				output.Write(decoded)
			case agentmgr.MsgTerminalClosed:
				var closed agentmgr.TerminalCloseData
				if err := json.Unmarshal(msg.Data, &closed); err != nil {
					t.Fatalf("decode terminal closed payload: %v", err)
				}
				if closed.SessionID != "sess-large-output" {
					t.Fatalf("expected closed session_id sess-large-output, got %q", closed.SessionID)
				}
				if !strings.Contains(output.String(), "BEGIN:") || !strings.Contains(output.String(), ":END") {
					t.Fatalf("expected large terminal output markers, got %q", output.String())
				}
				if dataFrames < 2 {
					t.Fatalf("expected large output to span multiple terminal.data frames, got %d", dataFrames)
				}
				if output.Len() < 6000 {
					t.Fatalf("expected collected output length >= 6000 bytes, got %d", output.Len())
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for large terminal output and close")
		}
	}
}

func TestTerminalManagerResizeUpdatesPTYSize(t *testing.T) {
	shellPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}

	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	tm := newTerminalManager()
	defer tm.CloseAll()

	startRaw, err := json.Marshal(agentmgr.TerminalStartData{
		SessionID: "sess-resize",
		Shell:     shellPath,
		Cols:      80,
		Rows:      24,
	})
	if err != nil {
		t.Fatalf("marshal terminal start: %v", err)
	}

	tm.HandleTerminalStart(transport, agentmgr.Message{Data: startRaw})
	_ = waitForCapturedAgentMessage(t, messages, agentmgr.MsgTerminalStarted, 5*time.Second)

	tm.Mu.Lock()
	sess := tm.Sessions["sess-resize"]
	tm.Mu.Unlock()
	if sess == nil {
		t.Fatal("expected terminal session to exist after start")
	}

	rows, cols, err := pty.Getsize(sess.Ptmx)
	if err != nil {
		t.Fatalf("get initial PTY size: %v", err)
	}
	if rows != 24 || cols != 80 {
		t.Fatalf("initial PTY size=%dx%d, want 24x80", rows, cols)
	}

	resizeRaw, err := json.Marshal(agentmgr.TerminalResizeData{
		SessionID: "sess-resize",
		Cols:      132,
		Rows:      51,
	})
	if err != nil {
		t.Fatalf("marshal terminal resize: %v", err)
	}
	tm.HandleTerminalResize(agentmgr.Message{Data: resizeRaw})

	rows, cols, err = pty.Getsize(sess.Ptmx)
	if err != nil {
		t.Fatalf("get resized PTY size: %v", err)
	}
	if rows != 51 || cols != 132 {
		t.Fatalf("resized PTY size=%dx%d, want 51x132", rows, cols)
	}

	closeRaw, err := json.Marshal(agentmgr.TerminalCloseData{SessionID: "sess-resize"})
	if err != nil {
		t.Fatalf("marshal terminal close: %v", err)
	}
	tm.HandleTerminalClose(agentmgr.Message{Data: closeRaw})

	closedMsg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgTerminalClosed, 5*time.Second)
	var closed agentmgr.TerminalCloseData
	if err := json.Unmarshal(closedMsg.Data, &closed); err != nil {
		t.Fatalf("decode terminal closed payload: %v", err)
	}
	if closed.SessionID != "sess-resize" {
		t.Fatalf("expected closed session_id sess-resize, got %q", closed.SessionID)
	}

	tm.Mu.Lock()
	_, exists := tm.Sessions["sess-resize"]
	tm.Mu.Unlock()
	if exists {
		t.Fatal("expected terminal session cleanup after explicit close")
	}
}

func TestTerminalManagerRejectsStartWhenSessionLimitReached(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	tm := newTerminalManager()
	for i := 0; i < maxTerminalSessions; i++ {
		tm.Sessions[string(rune('a'+i))] = nil
	}

	startRaw, err := json.Marshal(agentmgr.TerminalStartData{SessionID: "overflow"})
	if err != nil {
		t.Fatalf("marshal terminal start: %v", err)
	}
	tm.HandleTerminalStart(transport, agentmgr.Message{Data: startRaw})

	msg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgTerminalClosed, 2*time.Second)
	var closed agentmgr.TerminalCloseData
	if err := json.Unmarshal(msg.Data, &closed); err != nil {
		t.Fatalf("decode terminal closed payload: %v", err)
	}
	if !strings.Contains(closed.Reason, "max terminal sessions reached") {
		t.Fatalf("expected max-session rejection, got %q", closed.Reason)
	}
}

func TestFileManagerReadEmitsDataAndDone(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	root := resolvedTempDir(t)
	filePath := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(filePath, []byte("hello from file manager"), 0o644); err != nil {
		t.Fatalf("write sample file: %v", err)
	}

	fm := &files.Manager{
		BaseDir: root,
	}

	raw, err := json.Marshal(agentmgr.FileReadData{
		RequestID: "req-read",
		Path:      "sample.txt",
	})
	if err != nil {
		t.Fatalf("marshal file read request: %v", err)
	}
	fm.HandleFileRead(transport, agentmgr.Message{Data: raw})

	var content strings.Builder
	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg, ok := <-messages:
			if !ok {
				t.Fatal("transport closed before file read completed")
			}
			if msg.Type != agentmgr.MsgFileData {
				continue
			}
			var payload agentmgr.FileDataPayload
			if err := json.Unmarshal(msg.Data, &payload); err != nil {
				t.Fatalf("decode file data payload: %v", err)
			}
			decoded, err := base64.StdEncoding.DecodeString(payload.Data)
			if err != nil {
				t.Fatalf("decode file chunk: %v", err)
			}
			content.Write(decoded)
			if payload.Done {
				if payload.Error != "" {
					t.Fatalf("expected successful read, got error %q", payload.Error)
				}
				if content.String() != "hello from file manager" {
					t.Fatalf("unexpected file content %q", content.String())
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for file read result")
		}
	}
}

func TestFileManagerWriteCompletesAndPersistsFile(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	root := resolvedTempDir(t)
	fm := &files.Manager{
		BaseDir: root,
	}

	raw, err := json.Marshal(agentmgr.FileWriteData{
		RequestID: "req-write",
		Path:      "nested/output.txt",
		Data:      base64.StdEncoding.EncodeToString([]byte("written-by-agent")),
		Done:      true,
	})
	if err != nil {
		t.Fatalf("marshal file write request: %v", err)
	}
	fm.HandleFileWrite(transport, agentmgr.Message{Data: raw})

	msg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgFileWritten, 2*time.Second)
	var written agentmgr.FileWrittenData
	if err := json.Unmarshal(msg.Data, &written); err != nil {
		t.Fatalf("decode file written payload: %v", err)
	}
	if written.Error != "" {
		t.Fatalf("expected successful write, got error %q", written.Error)
	}
	if written.BytesWritten != int64(len("written-by-agent")) {
		t.Fatalf("unexpected bytes_written %d", written.BytesWritten)
	}

	data, err := os.ReadFile(filepath.Join(root, "nested/output.txt"))
	if err != nil {
		t.Fatalf("read persisted file: %v", err)
	}
	if string(data) != "written-by-agent" {
		t.Fatalf("unexpected persisted content %q", string(data))
	}

	if fm.HasPendingWrite("req-write") {
		t.Fatalf("expected pending writer cleanup after completed write")
	}
}

func TestFileManagerWriteFinalizesOnEmptyDoneMarker(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	root := resolvedTempDir(t)
	fm := &files.Manager{
		BaseDir: root,
	}

	firstChunk := strings.Repeat("w", files.FileChunkSize)
	firstRaw, err := json.Marshal(agentmgr.FileWriteData{
		RequestID: "req-write-boundary",
		Path:      "boundary/output.txt",
		Data:      base64.StdEncoding.EncodeToString([]byte(firstChunk)),
		Offset:    0,
		Done:      false,
	})
	if err != nil {
		t.Fatalf("marshal first file write request: %v", err)
	}
	fm.HandleFileWrite(transport, agentmgr.Message{Data: firstRaw})

	select {
	case msg := <-messages:
		t.Fatalf("unexpected response before terminal upload marker: %+v", msg)
	default:
	}

	finalRaw, err := json.Marshal(agentmgr.FileWriteData{
		RequestID: "req-write-boundary",
		Path:      "boundary/output.txt",
		Data:      "",
		Offset:    int64(len(firstChunk)),
		Done:      true,
	})
	if err != nil {
		t.Fatalf("marshal terminal file write request: %v", err)
	}
	fm.HandleFileWrite(transport, agentmgr.Message{Data: finalRaw})

	msg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgFileWritten, 2*time.Second)
	var written agentmgr.FileWrittenData
	if err := json.Unmarshal(msg.Data, &written); err != nil {
		t.Fatalf("decode file written payload: %v", err)
	}
	if written.Error != "" {
		t.Fatalf("expected successful write, got error %q", written.Error)
	}
	if written.BytesWritten != int64(len(firstChunk)) {
		t.Fatalf("unexpected bytes_written %d", written.BytesWritten)
	}

	data, err := os.ReadFile(filepath.Join(root, "boundary/output.txt"))
	if err != nil {
		t.Fatalf("read persisted file: %v", err)
	}
	if string(data) != firstChunk {
		t.Fatalf("unexpected persisted content length=%d", len(data))
	}

	tempFiles, err := filepath.Glob(filepath.Join(root, "boundary", ".lt-upload-*"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(tempFiles) != 0 {
		t.Fatalf("expected no leftover temp uploads, got %v", tempFiles)
	}

	if fm.HasPendingWrite("req-write-boundary") {
		t.Fatalf("expected pending writer cleanup after empty done marker")
	}
}

func TestFileManagerLifecycleCoversListMkdirRenameAndCopy(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	root := resolvedTempDir(t)
	fm := &files.Manager{
		BaseDir: root,
	}

	if err := os.WriteFile(filepath.Join(root, "source.txt"), []byte("source-data"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	mkdirRaw, err := json.Marshal(agentmgr.FileMkdirData{
		RequestID: "req-mkdir",
		Path:      "nested",
	})
	if err != nil {
		t.Fatalf("marshal file mkdir request: %v", err)
	}
	fm.HandleFileMkdir(transport, agentmgr.Message{Data: mkdirRaw})

	mkdirMsg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgFileResult, 2*time.Second)
	var mkdirResult agentmgr.FileResultData
	if err := json.Unmarshal(mkdirMsg.Data, &mkdirResult); err != nil {
		t.Fatalf("decode mkdir result payload: %v", err)
	}
	if !mkdirResult.OK || mkdirResult.Error != "" {
		t.Fatalf("expected mkdir success, got %+v", mkdirResult)
	}
	if _, err := os.Stat(filepath.Join(root, "nested")); err != nil {
		t.Fatalf("expected created directory: %v", err)
	}

	renameRaw, err := json.Marshal(agentmgr.FileRenameData{
		RequestID: "req-rename",
		OldPath:   "source.txt",
		NewPath:   "nested/renamed.txt",
	})
	if err != nil {
		t.Fatalf("marshal file rename request: %v", err)
	}
	fm.HandleFileRename(transport, agentmgr.Message{Data: renameRaw})

	renameMsg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgFileResult, 2*time.Second)
	var renameResult agentmgr.FileResultData
	if err := json.Unmarshal(renameMsg.Data, &renameResult); err != nil {
		t.Fatalf("decode rename result payload: %v", err)
	}
	if !renameResult.OK || renameResult.Error != "" {
		t.Fatalf("expected rename success, got %+v", renameResult)
	}
	if _, err := os.Stat(filepath.Join(root, "source.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected source.txt to be moved, stat err=%v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "nested", ".secret.txt"), []byte("hidden"), 0o644); err != nil {
		t.Fatalf("write hidden file: %v", err)
	}

	copyRaw, err := json.Marshal(agentmgr.FileCopyData{
		RequestID: "req-copy",
		SrcPath:   "nested/renamed.txt",
		DstPath:   "nested/copied.txt",
	})
	if err != nil {
		t.Fatalf("marshal file copy request: %v", err)
	}
	fm.HandleFileCopy(transport, agentmgr.Message{Data: copyRaw})

	copyMsg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgFileResult, 2*time.Second)
	var copyResult agentmgr.FileResultData
	if err := json.Unmarshal(copyMsg.Data, &copyResult); err != nil {
		t.Fatalf("decode copy result payload: %v", err)
	}
	if !copyResult.OK || copyResult.Error != "" {
		t.Fatalf("expected copy success, got %+v", copyResult)
	}

	renamedBytes, err := os.ReadFile(filepath.Join(root, "nested", "renamed.txt"))
	if err != nil {
		t.Fatalf("read renamed file: %v", err)
	}
	copiedBytes, err := os.ReadFile(filepath.Join(root, "nested", "copied.txt"))
	if err != nil {
		t.Fatalf("read copied file: %v", err)
	}
	if string(renamedBytes) != "source-data" || string(copiedBytes) != "source-data" {
		t.Fatalf("unexpected copied contents renamed=%q copied=%q", string(renamedBytes), string(copiedBytes))
	}

	listRaw, err := json.Marshal(agentmgr.FileListData{
		RequestID: "req-list",
		Path:      "nested",
	})
	if err != nil {
		t.Fatalf("marshal file list request: %v", err)
	}
	fm.HandleFileList(transport, agentmgr.Message{Data: listRaw})

	listMsg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgFileListed, 2*time.Second)
	var listed agentmgr.FileListedData
	if err := json.Unmarshal(listMsg.Data, &listed); err != nil {
		t.Fatalf("decode file listed payload: %v", err)
	}
	if listed.Error != "" {
		t.Fatalf("expected successful list, got error %q", listed.Error)
	}
	if listed.Path != filepath.Join(root, "nested") {
		t.Fatalf("listed path=%q, want %q", listed.Path, filepath.Join(root, "nested"))
	}
	if len(listed.Entries) != 2 {
		t.Fatalf("expected 2 visible entries, got %d", len(listed.Entries))
	}
	visibleNames := map[string]bool{}
	for _, entry := range listed.Entries {
		visibleNames[entry.Name] = true
		if strings.HasPrefix(entry.Name, ".") {
			t.Fatalf("unexpected hidden entry in visible list: %q", entry.Name)
		}
	}
	if !visibleNames["renamed.txt"] || !visibleNames["copied.txt"] {
		t.Fatalf("unexpected visible entries: %+v", listed.Entries)
	}

	showHiddenRaw, err := json.Marshal(agentmgr.FileListData{
		RequestID:  "req-list-hidden",
		Path:       "nested",
		ShowHidden: true,
	})
	if err != nil {
		t.Fatalf("marshal file list hidden request: %v", err)
	}
	fm.HandleFileList(transport, agentmgr.Message{Data: showHiddenRaw})

	hiddenMsg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgFileListed, 2*time.Second)
	var listedHidden agentmgr.FileListedData
	if err := json.Unmarshal(hiddenMsg.Data, &listedHidden); err != nil {
		t.Fatalf("decode hidden file listed payload: %v", err)
	}
	if listedHidden.Error != "" {
		t.Fatalf("expected successful hidden list, got error %q", listedHidden.Error)
	}
	if len(listedHidden.Entries) != 3 {
		t.Fatalf("expected 3 entries with hidden files shown, got %d", len(listedHidden.Entries))
	}
	hiddenNames := map[string]bool{}
	for _, entry := range listedHidden.Entries {
		hiddenNames[entry.Name] = true
	}
	if !hiddenNames[".secret.txt"] || !hiddenNames["renamed.txt"] || !hiddenNames["copied.txt"] {
		t.Fatalf("unexpected hidden-visible entries: %+v", listedHidden.Entries)
	}
}

func TestFileManagerDeleteRejectsBaseDirectory(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	root := resolvedTempDir(t)
	fm := &files.Manager{
		BaseDir: root,
	}

	raw, err := json.Marshal(agentmgr.FileDeleteData{
		RequestID: "req-delete",
		Path:      "",
	})
	if err != nil {
		t.Fatalf("marshal file delete request: %v", err)
	}
	fm.HandleFileDelete(transport, agentmgr.Message{Data: raw})

	msg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgFileResult, 2*time.Second)
	var result agentmgr.FileResultData
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		t.Fatalf("decode file result payload: %v", err)
	}
	if result.OK {
		t.Fatal("expected delete of base directory to be rejected")
	}
	if !strings.Contains(result.Error, "cannot delete base directory") {
		t.Fatalf("expected base-dir rejection, got %q", result.Error)
	}
}

func TestFileManagerSearchTruncatesAtMaxResults(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	root := resolvedTempDir(t)
	for _, name := range []string{"alpha.txt", "beta.txt", "gamma.txt", "ignore.log"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatalf("write search fixture %s: %v", name, err)
		}
	}

	fm := &files.Manager{
		BaseDir: root,
	}

	raw, err := json.Marshal(agentmgr.FileSearchData{
		RequestID:  "req-search",
		Path:       "",
		Pattern:    "*.txt",
		MaxResults: 2,
	})
	if err != nil {
		t.Fatalf("marshal file search request: %v", err)
	}
	fm.HandleFileSearch(transport, agentmgr.Message{Data: raw})

	msg := waitForCapturedAgentMessage(t, messages, agentmgr.MsgFileSearchResult, 2*time.Second)
	var result agentmgr.FileSearchResultData
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		t.Fatalf("decode file search payload: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("expected successful search, got error %q", result.Error)
	}
	if len(result.Matches) != 2 {
		t.Fatalf("expected 2 capped matches, got %d", len(result.Matches))
	}
	if !result.Truncated {
		t.Fatal("expected search result to be marked truncated")
	}
	for _, match := range result.Matches {
		if !strings.HasSuffix(match.Name, ".txt") {
			t.Fatalf("unexpected non-txt match %q", match.Name)
		}
		if !strings.HasPrefix(match.Path, root) {
			t.Fatalf("expected absolute path under base dir, got %q", match.Path)
		}
	}
}

func TestFileManagerReadLargeFileEmitsChunkSequenceAndDoneMarker(t *testing.T) {
	transport, messages, cleanup := newAgentcoreCapturedTransport(t)
	defer cleanup()

	root := resolvedTempDir(t)
	content := strings.Repeat("r", files.FileChunkSize*2)
	filePath := filepath.Join(root, "large.bin")
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write large file fixture: %v", err)
	}

	fm := &files.Manager{
		BaseDir: root,
	}

	raw, err := json.Marshal(agentmgr.FileReadData{
		RequestID: "req-large-read",
		Path:      "large.bin",
	})
	if err != nil {
		t.Fatalf("marshal file read request: %v", err)
	}
	fm.HandleFileRead(transport, agentmgr.Message{Data: raw})

	var (
		payloads []agentmgr.FileDataPayload
		data     strings.Builder
	)
	deadline := time.After(2 * time.Second)
readLoop:
	for len(payloads) < 3 {
		select {
		case msg, ok := <-messages:
			if !ok {
				t.Fatal("transport closed before large file read completed")
			}
			if msg.Type != agentmgr.MsgFileData {
				continue
			}
			var payload agentmgr.FileDataPayload
			if err := json.Unmarshal(msg.Data, &payload); err != nil {
				t.Fatalf("decode file data payload: %v", err)
			}
			payloads = append(payloads, payload)
			decoded, err := base64.StdEncoding.DecodeString(payload.Data)
			if err != nil {
				t.Fatalf("decode file chunk: %v", err)
			}
			data.Write(decoded)
			if payload.Done {
				break readLoop
			}
		case <-deadline:
			t.Fatal("timed out waiting for large file read results")
		}
	}

	if len(payloads) != 3 {
		t.Fatalf("expected 3 read payloads (2 chunks + done marker), got %d", len(payloads))
	}
	if payloads[0].Done || payloads[1].Done {
		t.Fatalf("expected first two payloads to be non-terminal: %+v", payloads)
	}
	if payloads[0].Offset != 0 || payloads[1].Offset != int64(files.FileChunkSize) || payloads[2].Offset != int64(files.FileChunkSize*2) {
		t.Fatalf("unexpected offsets: %+v", payloads)
	}
	if !payloads[2].Done || payloads[2].Data != "" || payloads[2].Error != "" {
		t.Fatalf("unexpected terminal payload: %+v", payloads[2])
	}
	if data.String() != content {
		t.Fatalf("unexpected reconstructed content length=%d want=%d", len(data.String()), len(content))
	}
}

func newAgentcoreCapturedTransport(t *testing.T) (*wsTransport, <-chan agentmgr.Message, func()) {
	t.Helper()

	serverConnCh := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		serverConnCh <- conn
	}))

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		server.Close()
		t.Fatalf("dial failed: %v", err)
	}

	var serverConn *websocket.Conn
	select {
	case serverConn = <-serverConnCh:
	case <-time.After(2 * time.Second):
		_ = clientConn.Close()
		server.Close()
		t.Fatal("timed out waiting for websocket capture connection")
	}

	messageCh := make(chan agentmgr.Message, 64)
	go func() {
		defer close(messageCh)
		for {
			var msg agentmgr.Message
			if err := serverConn.ReadJSON(&msg); err != nil {
				return
			}
			messageCh <- msg
		}
	}()

	cleanup := func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		server.Close()
	}
	return &wsTransport{conn: clientConn}, messageCh, cleanup
}

func waitForCapturedAgentMessage(t *testing.T, messages <-chan agentmgr.Message, wantType string, timeout time.Duration) agentmgr.Message {
	t.Helper()

	deadline := time.After(timeout)
	for {
		select {
		case msg, ok := <-messages:
			if !ok {
				t.Fatalf("capture channel closed before %s arrived", wantType)
			}
			if msg.Type == wantType {
				return msg
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s", wantType)
		}
	}
}

func resolvedTempDir(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("resolve temp dir %q: %v", root, err)
	}
	return resolved
}
