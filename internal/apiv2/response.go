package apiv2

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
)

func WriteJSON(w http.ResponseWriter, status int, data any) {
	markV2Envelope(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]any{
		"request_id": NewRequestID(),
		"data":       data,
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("apiv2: WriteJSON encode error: %v", err)
	}
}

func WriteList(w http.ResponseWriter, status int, data any, total, page, perPage int) {
	markV2Envelope(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]any{
		"request_id": NewRequestID(),
		"data":       data,
		"meta": map[string]int{
			"total":    total,
			"page":     page,
			"per_page": perPage,
		},
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("apiv2: WriteList encode error: %v", err)
	}
}

// WriteError writes a standard v2 JSON error response.
// For 5xx responses, the original message is logged server-side and a generic
// message is returned to the client to avoid leaking internal details.
func WriteError(w http.ResponseWriter, status int, errorCode, message string) {
	markV2Envelope(w)
	if status >= 500 {
		log.Printf("[error-sanitized] %d %s: %s", status, errorCode, message)
		message = "An internal error occurred."
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]any{
		"request_id": NewRequestID(),
		"error":      errorCode,
		"message":    message,
		"status":     status,
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("apiv2: WriteError encode error: %v", err)
	}
}

func NewRequestID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "req_" + hex.EncodeToString(b)
}

// WrapV1Handler wraps a v1 handler so its response is captured and re-wrapped
// in the v2 response envelope. The response is fully buffered in memory before
// forwarding — do NOT use this for streaming endpoints (file downloads, log
// tails, WebSocket upgrades). Those should use native v2 handlers or pass
// through directly.
//
// Responses written by this package's v2 writers pass through unchanged.
// Agent protocol responses can have request_id, data, or error fields, so
// their JSON shape cannot identify a v2 envelope. Non-JSON responses pass
// through unchanged.
func WrapV1Handler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Capture the v1 response.
		rec := &v2EnvelopeRecorder{ResponseRecorder: httptest.NewRecorder()}
		next(rec, r)

		result := rec.Result()
		body, _ := io.ReadAll(result.Body)
		if err := result.Body.Close(); err != nil {
			log.Printf("apiv2: closing wrapped response body: %v", err)
		}

		// Copy headers from the recorded response (before WriteHeader).
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}

		if rec.isV2Envelope {
			w.WriteHeader(rec.Code)
			_, _ = w.Write(body)
			return
		}

		// Only attempt to wrap JSON responses.
		if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
			var data any
			if err := json.Unmarshal(body, &data); err == nil {
				// Wrap error responses.
				if rec.Code >= 400 {
					errCode := "error"
					errMsg := ""
					if m, ok := data.(map[string]any); ok {
						if e, ok := m["error"].(string); ok && e != "" {
							errCode = e
							errMsg = e
						}
						// Check if there's a separate message field
						if msg, ok := m["message"].(string); ok && msg != "" {
							errMsg = msg
						}
					}
					WriteError(w, rec.Code, errCode, errMsg)
					return
				}

				// Wrap successful responses.
				WriteJSON(w, rec.Code, data)
				return
			}
		}

		// Non-JSON or unparseable — pass through as-is.
		w.WriteHeader(rec.Code)
		_, _ = w.Write(body)
	}
}

type v2EnvelopeRecorder struct {
	*httptest.ResponseRecorder
	isV2Envelope bool
}

func markV2Envelope(w http.ResponseWriter) {
	if recorder, ok := w.(*v2EnvelopeRecorder); ok {
		recorder.isV2Envelope = true
	}
}
