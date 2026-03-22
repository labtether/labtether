package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/audit"
)

// handleV2BulkServiceAction handles POST /api/v2/bulk/service-action.
func (s *apiServer) handleV2BulkServiceAction(w http.ResponseWriter, r *http.Request) {
	s.ensureBulkDeps().HandleV2BulkServiceAction(w, r)
}

// handleV2BulkFilePush pushes a file from a source connection to N destination
// connections in parallel, creating one file-transfer record per target.
//
// Request body:
//
//	{
//	  "source_connection_id": "conn_abc",
//	  "source_path":          "/tmp/payload.tar.gz",
//	  "targets": [
//	    { "dest_connection_id": "conn_def", "dest_path": "/tmp/payload.tar.gz" }
//	  ]
//	}
func (s *apiServer) handleV2BulkFilePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiv2.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if !apiv2.ScopeCheck(scopesFromContext(r.Context()), "files:write") {
		apiv2.WriteScopeForbidden(w, "files:write")
		return
	}

	var req struct {
		SourceConnectionID string `json:"source_connection_id"`
		SourcePath         string `json:"source_path"`
		Targets            []struct {
			DestConnectionID string `json:"dest_connection_id"`
			DestPath         string `json:"dest_path"`
		} `json:"targets"`
	}
	if err := decodeJSONBody(w, r, &req); err != nil {
		return
	}

	req.SourceConnectionID = strings.TrimSpace(req.SourceConnectionID)
	req.SourcePath = strings.TrimSpace(req.SourcePath)
	if req.SourceConnectionID == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "source_connection_id is required")
		return
	}
	if req.SourcePath == "" {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "source_path is required")
		return
	}
	if len(req.Targets) == 0 {
		apiv2.WriteError(w, http.StatusBadRequest, "validation", "targets must not be empty")
		return
	}

	type targetResult struct {
		DestConnectionID string `json:"dest_connection_id"`
		DestPath         string `json:"dest_path"`
		TransferID       string `json:"transfer_id,omitempty"`
		Error            string `json:"error,omitempty"`
	}

	results := make([]targetResult, len(req.Targets))
	var mu sync.Mutex
	var wg sync.WaitGroup
	requestCtx := context.WithoutCancel(r.Context())

	for i, t := range req.Targets {
		wg.Add(1)
		go func(idx int, destConnID, destPath string) {
			defer wg.Done()

			res := targetResult{
				DestConnectionID: strings.TrimSpace(destConnID),
				DestPath:         strings.TrimSpace(destPath),
			}

			if res.DestConnectionID == "" || res.DestPath == "" {
				res.Error = "dest_connection_id and dest_path are required for each target"
				mu.Lock()
				results[idx] = res
				mu.Unlock()
				return
			}

			// Build a JSON body matching what the v1 file-transfer handler expects.
			bodyMap := map[string]string{
				"source_type": "connection",
				"source_id":   req.SourceConnectionID,
				"source_path": req.SourcePath,
				"dest_type":   "connection",
				"dest_id":     res.DestConnectionID,
				"dest_path":   res.DestPath,
			}
			bodyBytes, err := json.Marshal(bodyMap)
			if err != nil {
				res.Error = "internal: failed to marshal transfer body: " + err.Error()
				mu.Lock()
				results[idx] = res
				mu.Unlock()
				return
			}

			syntheticReq, err := http.NewRequestWithContext(
				requestCtx,
				http.MethodPost,
				"/api/v1/file-transfers",
				strings.NewReader(string(bodyBytes)),
			)
			if err != nil {
				res.Error = "internal: failed to construct transfer request: " + err.Error()
				mu.Lock()
				results[idx] = res
				mu.Unlock()
				return
			}
			syntheticReq.Header.Set("Content-Type", "application/json")

			rec := httptest.NewRecorder()
			// Use the raw v1 handler directly — WrapV1Handler would re-wrap the
			// response, but we only need the transfer ID from the raw JSON.
			s.handleFileTransfers(rec, syntheticReq)

			if rec.Code >= 400 {
				// Extract error message from the v1 response if possible.
				var errBody map[string]any
				if json.Unmarshal(rec.Body.Bytes(), &errBody) == nil {
					if msg, ok := errBody["error"].(string); ok && msg != "" {
						res.Error = msg
					} else {
						res.Error = "file transfer failed with status " + http.StatusText(rec.Code)
					}
				} else {
					res.Error = "file transfer failed with status " + http.StatusText(rec.Code)
				}
			} else {
				// Extract the transfer ID from {"transfer": {"id": "..."}} response.
				var respBody struct {
					Transfer struct {
						ID string `json:"id"`
					} `json:"transfer"`
				}
				if json.Unmarshal(rec.Body.Bytes(), &respBody) == nil && respBody.Transfer.ID != "" {
					res.TransferID = respBody.Transfer.ID
				}
			}

			mu.Lock()
			results[idx] = res
			mu.Unlock()
		}(i, t.DestConnectionID, t.DestPath)
	}
	wg.Wait()

	s.appendAuditEventBestEffort(audit.Event{
		Type:      "api.bulk.file_push",
		ActorID:   principalActorID(r.Context()),
		Details:   map[string]any{"source_connection_id": req.SourceConnectionID, "source_path": req.SourcePath, "target_count": len(req.Targets)},
		Timestamp: time.Now().UTC(),
	}, "v2 bulk file-push")

	apiv2.WriteJSON(w, http.StatusOK, map[string]any{"results": results})
}
