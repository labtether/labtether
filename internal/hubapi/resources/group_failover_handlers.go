package resources

import (
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/groupfailover"
	"github.com/labtether/labtether/internal/servicehttp"
)

func (d *Deps) HandleFailoverPairs(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/group-failover-pairs" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.FailoverStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "failover store unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		pairs, err := d.FailoverStore.ListFailoverPairs(parseLimit(r, 50))
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list failover pairs")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"pairs": pairs})
	case http.MethodPost:
		if !d.EnforceRateLimit(w, r, "failover.create", 120, time.Minute) {
			return
		}
		var body struct {
			PrimaryGroupID string `json:"primary_group_id"`
			BackupGroupID  string `json:"backup_group_id"`
			Name           string `json:"name,omitempty"`
		}
		if err := d.DecodeJSONBody(w, r, &body); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid failover pair payload")
			return
		}
		req := groupfailover.CreatePairRequest{
			PrimaryGroupID: strings.TrimSpace(body.PrimaryGroupID),
			BackupGroupID:  strings.TrimSpace(body.BackupGroupID),
			Name:           strings.TrimSpace(body.Name),
		}
		if req.PrimaryGroupID == "" || req.BackupGroupID == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "primary_group_id and backup_group_id are required")
			return
		}
		if req.PrimaryGroupID == req.BackupGroupID {
			servicehttp.WriteError(w, http.StatusBadRequest, "primary_group_id and backup_group_id must be different")
			return
		}
		pair, err := d.FailoverStore.CreateFailoverPair(req)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate") ||
				strings.Contains(strings.ToLower(err.Error()), "unique") {
				servicehttp.WriteError(w, http.StatusConflict, "failover pair already exists")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create failover pair")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{"pair": pair})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (d *Deps) HandleFailoverPairActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/group-failover-pairs/")
	if path == r.URL.Path || path == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "failover pair path not found")
		return
	}
	if d.FailoverStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "failover store unavailable")
		return
	}

	parts := strings.Split(path, "/")
	pairID := strings.TrimSpace(parts[0])
	if pairID == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "failover pair path not found")
		return
	}

	// POST /group-failover-pairs/{id}/check-readiness
	if len(parts) == 2 && parts[1] == "check-readiness" {
		if r.Method != http.MethodPost {
			servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		pair, ok, err := d.FailoverStore.GetFailoverPair(pairID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load failover pair")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "failover pair not found")
			return
		}
		score := 0
		if d.GroupStore != nil {
			if _, ok, err := d.GroupStore.GetGroup(pair.PrimaryGroupID); err == nil && ok {
				score += 50
			}
			if _, ok, err := d.GroupStore.GetGroup(pair.BackupGroupID); err == nil && ok {
				score += 50
			}
		}
		now := time.Now().UTC()
		if err := d.FailoverStore.UpdateFailoverReadiness(pairID, score, now); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update readiness score")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
			"pair_id":         pairID,
			"readiness_score": score,
			"checked_at":      now,
		})
		return
	}

	if len(parts) > 1 {
		servicehttp.WriteError(w, http.StatusNotFound, "unknown failover pair action")
		return
	}

	// GET/PUT/DELETE /group-failover-pairs/{id}
	switch r.Method {
	case http.MethodGet:
		pair, ok, err := d.FailoverStore.GetFailoverPair(pairID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load failover pair")
			return
		}
		if !ok {
			servicehttp.WriteError(w, http.StatusNotFound, "failover pair not found")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"pair": pair})
	case http.MethodPut, http.MethodPatch:
		var body struct {
			PrimaryGroupID *string `json:"primary_group_id,omitempty"`
			BackupGroupID  *string `json:"backup_group_id,omitempty"`
			Name           *string `json:"name,omitempty"`
		}
		if err := d.DecodeJSONBody(w, r, &body); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid failover pair payload")
			return
		}
		req := groupfailover.UpdatePairRequest{
			Name: body.Name,
		}
		if body.PrimaryGroupID != nil {
			req.PrimaryGroupID = body.PrimaryGroupID
		}
		if body.BackupGroupID != nil {
			req.BackupGroupID = body.BackupGroupID
		}
		updated, err := d.FailoverStore.UpdateFailoverPair(pairID, req)
		if err != nil {
			if err == groupfailover.ErrPairNotFound {
				servicehttp.WriteError(w, http.StatusNotFound, "failover pair not found")
				return
			}
			if strings.Contains(strings.ToLower(err.Error()), "must be different") {
				servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update failover pair")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"pair": updated})
	case http.MethodDelete:
		if err := d.FailoverStore.DeleteFailoverPair(pairID); err != nil {
			if err == groupfailover.ErrPairNotFound {
				servicehttp.WriteError(w, http.StatusNotFound, "failover pair not found")
				return
			}
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete failover pair")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
