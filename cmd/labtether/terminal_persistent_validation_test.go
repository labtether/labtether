package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/terminal"
)

func TestPersistentSessionCreateRejectsOversizedTargetAndTitle(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]string
	}{
		{
			name: "target",
			payload: map[string]string{
				"target": strings.Repeat("a", maxTargetLength+1),
				"title":  "Shell",
			},
		},
		{
			name: "title",
			payload: map[string]string{
				"target": "lab-host-01",
				"title":  strings.Repeat("a", maxTargetLength+1),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sut := newTestAPIServer(t)
			body, err := json.Marshal(test.payload)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/terminal/persistent-sessions", bytes.NewReader(body))
			request = request.WithContext(contextWithUserID(request.Context(), "actor-a"))
			recorder := httptest.NewRecorder()

			sut.handlePersistentSessions(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
		})
	}
}

func TestPersistentSessionUpdateRejectsOversizedTitle(t *testing.T) {
	sut := newTestAPIServer(t)
	createRequest := httptest.NewRequest(
		http.MethodPost,
		"/terminal/persistent-sessions",
		bytes.NewReader([]byte(`{"target":"lab-host-01","title":"Shell"}`)),
	)
	createRequest = createRequest.WithContext(contextWithUserID(createRequest.Context(), "actor-a"))
	createRecorder := httptest.NewRecorder()
	sut.handlePersistentSessions(createRecorder, createRequest)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d; body=%s", createRecorder.Code, http.StatusCreated, createRecorder.Body.String())
	}

	var created struct {
		PersistentSession terminal.PersistentSession `json:"persistent_session"`
	}
	if err := json.Unmarshal(createRecorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	updateBody, err := json.Marshal(map[string]string{"title": strings.Repeat("a", maxTargetLength+1)})
	if err != nil {
		t.Fatal(err)
	}
	updateRequest := httptest.NewRequest(
		http.MethodPut,
		"/terminal/persistent-sessions/"+created.PersistentSession.ID,
		bytes.NewReader(updateBody),
	)
	updateRequest = updateRequest.WithContext(contextWithUserID(updateRequest.Context(), "actor-a"))
	updateRecorder := httptest.NewRecorder()

	sut.handlePersistentSessionActions(updateRecorder, updateRequest)

	if updateRecorder.Code != http.StatusBadRequest {
		t.Fatalf("update status = %d, want %d; body=%s", updateRecorder.Code, http.StatusBadRequest, updateRecorder.Body.String())
	}
}
