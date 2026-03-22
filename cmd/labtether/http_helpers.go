package main

import (
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/updates"
)

// Thin aliases delegating to internal/hubapi/shared so that callers
// inside cmd/labtether/ keep compiling without a mass rename.

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) error {
	return shared.DecodeJSONBody(w, r, dst)
}

func mapCommandLevel(status string) string { return shared.MapCommandLevel(status) }

func parseLimit(r *http.Request, fallback int) int { return shared.ParseLimit(r, fallback) }

func parseOffset(r *http.Request) int { return shared.ParseOffset(r) }

func groupIDQueryParam(r *http.Request) string { return shared.GroupIDQueryParam(r) }

func parseTimestampParam(raw string, fallback time.Time) time.Time {
	return shared.ParseTimestampParam(raw, fallback)
}

func parseDurationParam(raw string, fallback, min, max time.Duration) time.Duration {
	return shared.ParseDurationParam(raw, fallback, min, max)
}

func defaultStepForWindow(window time.Duration) time.Duration {
	return shared.DefaultStepForWindow(window)
}

func requestClientKey(r *http.Request) string { return shared.RequestClientKey(r) }

func validateMaxLen(field, value string, maxLen int) error {
	return shared.ValidateMaxLen(field, value, maxLen)
}

func actionRunMatchesGroup(run actions.Run, groupID string, assetGroup map[string]string) bool {
	return shared.ActionRunMatchesGroup(run, groupID, assetGroup)
}

func filterLogEventsByGroup(events []logs.Event, groupID string, assetGroup map[string]string) []logs.Event {
	return shared.FilterLogEventsByGroup(events, groupID, assetGroup)
}

func groupAssetIDsForGroup(groupID string, assetGroup map[string]string) []string {
	return shared.GroupAssetIDsForGroup(groupID, assetGroup)
}

func updatePlanTouchesGroup(plan updates.Plan, groupID string, assetGroup map[string]string) bool {
	return shared.UpdatePlanTouchesGroup(plan, groupID, assetGroup)
}

// apiServer methods that reference server state stay here.

func (s *apiServer) enforceRateLimit(w http.ResponseWriter, r *http.Request, bucket string, limit int, window time.Duration) bool {
	if limit <= 0 || window <= 0 {
		return true
	}

	now := time.Now().UTC()
	key := bucket + ":" + requestClientKey(r)

	s.rateLimiter.Mu.Lock()
	if s.rateLimiter.Windows == nil {
		s.rateLimiter.Windows = make(map[string]rateCounter, 64)
	}

	// Prune expired entries at most once per minute to prevent unbounded map
	// growth without blocking every request with a full map scan.
	if len(s.rateLimiter.Windows) > 100 && now.After(s.rateLimiter.PrunedAt.Add(time.Minute)) {
		for k, v := range s.rateLimiter.Windows {
			if now.After(v.ResetAt) {
				delete(s.rateLimiter.Windows, k)
			}
		}
		s.rateLimiter.PrunedAt = now
	}

	counter := s.rateLimiter.Windows[key]
	if counter.ResetAt.IsZero() || now.After(counter.ResetAt) {
		counter = rateCounter{
			Count:   0,
			ResetAt: now.Add(window),
		}
	}
	if counter.Count >= limit {
		s.rateLimiter.Windows[key] = counter
		s.rateLimiter.Mu.Unlock()
		servicehttp.WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return false
	}
	counter.Count++
	s.rateLimiter.Windows[key] = counter
	s.rateLimiter.Mu.Unlock()

	return true
}

// enforceRateLimitGlobal is like enforceRateLimit but uses the bucket key as-is
// without appending the client IP. This allows rate limiting across all IPs for
// a given principal (e.g. an API key regardless of which IP it originates from).
func (s *apiServer) enforceRateLimitGlobal(w http.ResponseWriter, bucket string, limit int, window time.Duration) bool {
	if limit <= 0 || window <= 0 {
		return true
	}

	now := time.Now().UTC()
	key := bucket

	s.rateLimiter.Mu.Lock()
	if s.rateLimiter.Windows == nil {
		s.rateLimiter.Windows = make(map[string]rateCounter, 64)
	}

	if len(s.rateLimiter.Windows) > 100 && now.After(s.rateLimiter.PrunedAt.Add(time.Minute)) {
		for k, v := range s.rateLimiter.Windows {
			if now.After(v.ResetAt) {
				delete(s.rateLimiter.Windows, k)
			}
		}
		s.rateLimiter.PrunedAt = now
	}

	counter := s.rateLimiter.Windows[key]
	if counter.ResetAt.IsZero() || now.After(counter.ResetAt) {
		counter = rateCounter{
			Count:   0,
			ResetAt: now.Add(window),
		}
	}
	if counter.Count >= limit {
		s.rateLimiter.Windows[key] = counter
		s.rateLimiter.Mu.Unlock()
		servicehttp.WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return false
	}
	counter.Count++
	s.rateLimiter.Windows[key] = counter
	s.rateLimiter.Mu.Unlock()

	return true
}

// loadUpdatePlansByID loads update plans by their IDs and returns a map keyed by
// plan ID for fast lookup.
func (s *apiServer) loadUpdatePlansByID(planIDs []string) (map[string]updates.Plan, error) {
	out := make(map[string]updates.Plan, len(planIDs))
	if s.updateStore == nil || len(planIDs) == 0 {
		return out, nil
	}
	for _, id := range planIDs {
		if _, exists := out[id]; exists {
			continue
		}
		plan, ok, err := s.updateStore.GetUpdatePlan(id)
		if err != nil {
			return out, err
		}
		if ok {
			out[id] = plan
		}
	}
	return out, nil
}

// updateRunTouchesGroup checks if an update run's plan targets assets in the
// specified group. Results are cached in planGroupCache for deduplication.
func (s *apiServer) updateRunTouchesGroup(planID, groupID string, assetGroup map[string]string, planGroupCache map[string]bool) (bool, error) {
	if groupID == "" {
		return true, nil
	}
	if cached, ok := planGroupCache[planID]; ok {
		return cached, nil
	}
	if s.updateStore == nil {
		return false, nil
	}
	plan, ok, err := s.updateStore.GetUpdatePlan(planID)
	if err != nil {
		return false, err
	}
	if !ok {
		planGroupCache[planID] = false
		return false, nil
	}
	touches := updatePlanTouchesGroup(plan, groupID, assetGroup)
	planGroupCache[planID] = touches
	return touches, nil
}
