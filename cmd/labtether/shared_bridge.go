package main

import (
	"net/http"

	"github.com/labtether/labtether/internal/hubapi/shared"
)

// buildDeadLetterDeps constructs shared.DeadLetterDeps from the apiServer's fields.
func (s *apiServer) buildDeadLetterDeps() *shared.DeadLetterDeps {
	return &shared.DeadLetterDeps{
		LogStore: s.logStore,
	}
}

// handleDeadLetters delegates to the shared package's DeadLetterDeps handler.
func (s *apiServer) handleDeadLetters(w http.ResponseWriter, r *http.Request) {
	s.buildDeadLetterDeps().HandleDeadLetters(w, r)
}
