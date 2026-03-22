package files

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
)

const (
	FileChunkSize           = 64 * 1024         // 64KB per chunk
	MaxFileSize             = 512 * 1024 * 1024 // 512MB max file transfer
	maxWritePending         = 64
	orphanCleanupScanBudget = 2 * time.Second
	orphanCleanupMaxEntries = 20000
)

// MessageSender abstracts the agent-to-hub send capability so this package
// does not depend on the concrete wsTransport type in the parent agentcore package.
type MessageSender interface {
	Send(msg agentmgr.Message) error
}

// Manager manages file operations on the agent.
type Manager struct {
	mu      sync.Mutex
	writers map[string]*PendingWrite // request_id -> pending write
	BaseDir string                   // restricted base directory (empty = home dir)
	HomeDir string                   // resolved home directory for "~" expansion
}

// PendingWrite tracks an in-progress file upload.
type PendingWrite struct {
	File    *os.File
	Path    string
	TmpPath string
	Written int64
}

// NewManager creates a new file Manager with the given file root mode.
func NewManager(fileRootMode string) *Manager {
	homeDir := ResolveAgentFileHomeDir()
	fm := &Manager{
		writers: make(map[string]*PendingWrite),
		BaseDir: ResolveFileBaseDirWithHome(fileRootMode, homeDir),
		HomeDir: homeDir,
	}
	go fm.CleanupOrphanedTempFiles()
	return fm
}

// HandleFileList handles a file list request from the hub.
func (fm *Manager) HandleFileList(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.FileListData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("file: invalid list request: %v", err)
		return
	}

	dirPath, err := fm.ValidatePath(req.Path)
	if err != nil {
		fm.sendFileListed(transport, req.RequestID, req.Path, nil, err.Error())
		return
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		fm.sendFileListed(transport, req.RequestID, dirPath, nil, err.Error())
		return
	}

	var fileEntries []agentmgr.FileEntry
	for _, entry := range entries {
		if !req.ShowHidden && strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		fileEntries = append(fileEntries, agentmgr.FileEntry{
			Name:    entry.Name(),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
			IsDir:   entry.IsDir(),
		})
	}

	fm.sendFileListed(transport, req.RequestID, dirPath, fileEntries, "")
}

func (fm *Manager) sendFileListed(transport MessageSender, requestID, path string, entries []agentmgr.FileEntry, errMsg string) {
	if entries == nil {
		entries = []agentmgr.FileEntry{}
	}
	data, _ := json.Marshal(agentmgr.FileListedData{
		RequestID: requestID,
		Path:      path,
		Entries:   entries,
		Error:     errMsg,
	})
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgFileListed,
		ID:   requestID,
		Data: data,
	})
}

// HandleFileRead handles a file read request from the hub.
func (fm *Manager) HandleFileRead(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.FileReadData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("file: invalid read request: %v", err)
		return
	}

	filePath, err := fm.ValidatePath(req.Path)
	if err != nil {
		fm.sendFileData(transport, req.RequestID, "", 0, true, err.Error())
		return
	}

	info, err := os.Stat(filePath)
	if err != nil {
		fm.sendFileData(transport, req.RequestID, "", 0, true, err.Error())
		return
	}
	if info.IsDir() {
		fm.sendFileData(transport, req.RequestID, "", 0, true, "cannot read a directory")
		return
	}
	if info.Size() > MaxFileSize {
		fm.sendFileData(transport, req.RequestID, "", 0, true, "file too large")
		return
	}

	f, err := os.Open(filePath) // #nosec G304 -- Agent file operations intentionally target operator-requested local paths after authz.
	if err != nil {
		fm.sendFileData(transport, req.RequestID, "", 0, true, err.Error())
		return
	}
	defer f.Close()

	buf := make([]byte, FileChunkSize)
	var offset int64
	for {
		n, readErr := f.Read(buf)
		if n > 0 {
			encoded := base64.StdEncoding.EncodeToString(buf[:n])
			done := readErr == io.EOF
			fm.sendFileData(transport, req.RequestID, encoded, offset, done, "")
			offset += int64(n)
			if done {
				return
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				fm.sendFileData(transport, req.RequestID, "", offset, true, readErr.Error())
			} else {
				// EOF with n==0 -- send final empty done marker.
				fm.sendFileData(transport, req.RequestID, "", offset, true, "")
			}
			return
		}
	}
}

func (fm *Manager) sendFileData(transport MessageSender, requestID, data string, offset int64, done bool, errMsg string) {
	payload, _ := json.Marshal(agentmgr.FileDataPayload{
		RequestID: requestID,
		Data:      data,
		Offset:    offset,
		Done:      done,
		Error:     errMsg,
	})
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgFileData,
		ID:   requestID,
		Data: payload,
	})
}

// SendFileWritten sends a file-written acknowledgement to the hub.
func (fm *Manager) SendFileWritten(transport MessageSender, requestID string, bytesWritten int64, errMsg string) {
	data, _ := json.Marshal(agentmgr.FileWrittenData{
		RequestID:    requestID,
		BytesWritten: bytesWritten,
		Error:        errMsg,
	})
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgFileWritten,
		ID:   requestID,
		Data: data,
	})
}

// HandleFileMkdir handles a mkdir request from the hub.
func (fm *Manager) HandleFileMkdir(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.FileMkdirData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("file: invalid mkdir request: %v", err)
		return
	}

	dirPath, err := fm.ValidatePath(req.Path)
	if err != nil {
		fm.SendFileResult(transport, req.RequestID, false, err.Error())
		return
	}

	if err := os.MkdirAll(dirPath, 0o750); err != nil {
		fm.SendFileResult(transport, req.RequestID, false, err.Error())
		return
	}

	fm.SendFileResult(transport, req.RequestID, true, "")
}

// HandleFileDelete handles a file delete request from the hub.
func (fm *Manager) HandleFileDelete(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.FileDeleteData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("file: invalid delete request: %v", err)
		return
	}

	filePath, err := fm.ValidatePath(req.Path)
	if err != nil {
		fm.SendFileResult(transport, req.RequestID, false, err.Error())
		return
	}

	// Safety: don't allow deleting the base directory itself.
	if filePath == fm.BaseDir {
		fm.SendFileResult(transport, req.RequestID, false, "cannot delete base directory")
		return
	}

	if err := os.RemoveAll(filePath); err != nil {
		fm.SendFileResult(transport, req.RequestID, false, err.Error())
		return
	}

	fm.SendFileResult(transport, req.RequestID, true, "")
}

// HandleFileRename handles a file rename request from the hub.
func (fm *Manager) HandleFileRename(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.FileRenameData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("file: invalid rename request: %v", err)
		return
	}

	oldPath, err := fm.ValidatePath(req.OldPath)
	if err != nil {
		fm.SendFileResult(transport, req.RequestID, false, err.Error())
		return
	}

	newPath, err := fm.ValidatePath(req.NewPath)
	if err != nil {
		fm.SendFileResult(transport, req.RequestID, false, err.Error())
		return
	}

	// Ensure source exists.
	if _, err := os.Stat(oldPath); err != nil {
		fm.SendFileResult(transport, req.RequestID, false, err.Error())
		return
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		fm.SendFileResult(transport, req.RequestID, false, err.Error())
		return
	}

	fm.SendFileResult(transport, req.RequestID, true, "")
}

// SendFileResult sends a generic file operation result to the hub.
func (fm *Manager) SendFileResult(transport MessageSender, requestID string, ok bool, errMsg string) {
	data, _ := json.Marshal(agentmgr.FileResultData{
		RequestID: requestID,
		OK:        ok,
		Error:     errMsg,
	})
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgFileResult,
		ID:   requestID,
		Data: data,
	})
}

// SkipSearchDirs returns true for directories that should be skipped during file search.
var SkipSearchDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	".cache":       true,
}

// HandleFileSearch performs a recursive filename search under a base path,
// matching filenames against a glob pattern. Results are capped at MaxResults
// (default 100, max 500) and the walk is bounded to a 10-second context timeout.
func (fm *Manager) HandleFileSearch(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.FileSearchData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("file: invalid search request: %v", err)
		return
	}

	sendResult := func(matches []agentmgr.FileEntry, truncated bool, errMsg string) {
		if matches == nil {
			matches = []agentmgr.FileEntry{}
		}
		data, _ := json.Marshal(agentmgr.FileSearchResultData{
			RequestID: req.RequestID,
			Matches:   matches,
			Error:     errMsg,
			Truncated: truncated,
		})
		_ = transport.Send(agentmgr.Message{
			Type: agentmgr.MsgFileSearchResult,
			ID:   req.RequestID,
			Data: data,
		})
	}

	searchPath, err := fm.ValidatePath(req.Path)
	if err != nil {
		sendResult(nil, false, err.Error())
		return
	}

	// Apply MaxResults bounds: default 100, cap at 500.
	maxResults := req.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}
	if maxResults > 500 {
		maxResults = 500
	}

	// Use empty pattern as a match-all wildcard.
	pattern := req.Pattern
	if strings.TrimSpace(pattern) == "" {
		pattern = "*"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var matches []agentmgr.FileEntry
	truncated := false

	walkErr := filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
		// Respect timeout.
		if ctx.Err() != nil {
			return filepath.SkipAll
		}

		if err != nil {
			return nil // skip inaccessible paths
		}

		// Skip excluded directories in-place.
		if d.IsDir() && SkipSearchDirs[d.Name()] {
			return filepath.SkipDir
		}

		// Only match filenames (not the root search path itself).
		if path == searchPath {
			return nil
		}

		matched, matchErr := filepath.Match(pattern, d.Name())
		if matchErr != nil {
			// Invalid pattern -- abort walk and surface the error.
			return matchErr
		}
		if !matched {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}

		matches = append(matches, agentmgr.FileEntry{
			Name:    d.Name(),
			Path:    path,
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime().UTC().Format(time.RFC3339),
			IsDir:   d.IsDir(),
		})

		if len(matches) >= maxResults {
			truncated = true
			return filepath.SkipAll
		}

		return nil
	})

	if walkErr != nil && !errors.Is(walkErr, filepath.SkipAll) {
		sendResult(matches, truncated, walkErr.Error())
		return
	}

	// Timeout hit: mark as truncated.
	if ctx.Err() != nil {
		truncated = true
	}

	sendResult(matches, truncated, "")
}
