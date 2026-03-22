package main

import (
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/actions"
	groupfeaturespkg "github.com/labtether/labtether/internal/hubapi/groupfeatures"
)

// handleGroupMaintenanceWindowsCollection forwards to groupfeatures.Deps.
func (s *apiServer) handleGroupMaintenanceWindowsCollection(w http.ResponseWriter, r *http.Request, groupID string) {
	s.ensureGroupFeaturesDeps().HandleGroupMaintenanceWindowsCollection(w, r, groupID)
}

// handleGroupMaintenanceWindowActions forwards to groupfeatures.Deps.
func (s *apiServer) handleGroupMaintenanceWindowActions(w http.ResponseWriter, r *http.Request, groupID, windowID string) {
	s.ensureGroupFeaturesDeps().HandleGroupMaintenanceWindowActions(w, r, groupID, windowID)
}

// resolveGroupIDForAction forwards to groupfeatures.Deps.
func (s *apiServer) resolveGroupIDForAction(req actions.ExecuteRequest) (string, error) {
	return s.ensureGroupFeaturesDeps().ResolveGroupIDForAction(req)
}

// resolveGroupIDsForTargets forwards to groupfeatures.Deps.
func (s *apiServer) resolveGroupIDsForTargets(targets []string) (map[string]struct{}, error) {
	return s.ensureGroupFeaturesDeps().ResolveGroupIDsForTargets(targets)
}

// groupGuardrails forwards to groupfeatures.Deps.
func (s *apiServer) groupGuardrails(groupID string, at time.Time) (groupfeaturespkg.GroupMaintenanceGuardrails, error) {
	return s.ensureGroupFeaturesDeps().EvaluateGuardrails(groupID, at)
}
