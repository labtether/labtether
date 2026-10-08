package alerting

import (
	"context"
	"github.com/labtether/labtether/internal/persistence"
	"sync"
	"time"
)

type liveActivityStoreStub struct {
	mu        sync.Mutex
	records   map[string]persistence.LiveActivityPushToken
	reconcile map[string]bool
	upsertErr error
}

func newLiveActivityStoreStub() *liveActivityStoreStub {
	return &liveActivityStoreStub{
		records:   make(map[string]persistence.LiveActivityPushToken),
		reconcile: make(map[string]bool),
	}
}

func (s *liveActivityStoreStub) UpsertLiveActivityPushToken(_ context.Context, token persistence.LiveActivityPushToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.upsertErr != nil {
		return s.upsertErr
	}
	for id, existing := range s.records {
		if (existing.UserID == token.UserID && existing.DeviceID == token.DeviceID && existing.ActivityID == token.ActivityID) ||
			(existing.TokenHash == token.TokenHash && existing.BundleID == token.BundleID && existing.Environment == token.Environment) {
			delete(s.records, id)
		}
	}
	s.records[token.ID] = token
	return nil
}

func (s *liveActivityStoreStub) DeleteLiveActivityPushTokenByOwnerAndID(
	_ context.Context,
	userID, deviceID, activityID, incidentID, registrationID string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, found := s.records[registrationID]
	if found && token.UserID == userID && token.DeviceID == deviceID &&
		token.ActivityID == activityID && token.IncidentID == incidentID {
		delete(s.records, registrationID)
	}
	return nil
}

func (s *liveActivityStoreStub) DeleteLiveActivityPushToken(_ context.Context, userID, deviceID, activityID, incidentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, token := range s.records {
		if token.UserID == userID && token.DeviceID == deviceID && token.ActivityID == activityID && token.IncidentID == incidentID {
			delete(s.records, id)
		}
	}
	return nil
}

func (s *liveActivityStoreStub) DeleteLiveActivityPushTokenByID(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, id)
	return nil
}

func (s *liveActivityStoreStub) DeleteLiveActivityPushTokenByGeneration(_ context.Context, id string, generation int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if token, found := s.records[id]; found && token.DeliveryGeneration == generation {
		delete(s.records, id)
	}
	return nil
}

func (s *liveActivityStoreStub) ListLiveActivityPushTokensForIncident(_ context.Context, incidentID string, now time.Time) ([]persistence.LiveActivityPushToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]persistence.LiveActivityPushToken, 0)
	for _, token := range s.records {
		if token.IncidentID == incidentID && token.ExpiresAt.After(now) {
			result = append(result, token)
		}
	}
	return result, nil
}

func (s *liveActivityStoreStub) ListDueLiveActivityPushTokens(_ context.Context, now time.Time, limit int) ([]persistence.LiveActivityPushToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]persistence.LiveActivityPushToken, 0)
	for _, token := range s.records {
		if token.ExpiresAt.After(now) && token.NextRetryAt != nil && !token.NextRetryAt.After(now) {
			result = append(result, token)
			if len(result) == limit {
				break
			}
		}
	}
	return result, nil
}

func (s *liveActivityStoreStub) ListLiveActivityPushTokensForReconciliation(_ context.Context, now time.Time, limit int) ([]persistence.LiveActivityPushToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]persistence.LiveActivityPushToken, 0)
	for id, token := range s.records {
		if s.reconcile[id] && token.ExpiresAt.After(now) && (token.NextRetryAt == nil || !token.NextRetryAt.After(now)) {
			result = append(result, token)
			delete(s.reconcile, id)
			if len(result) == limit {
				break
			}
		}
	}
	return result, nil
}

func (s *liveActivityStoreStub) ClaimLiveActivityPushDelivery(
	_ context.Context,
	id string,
	expectedGeneration int64,
	pendingState string,
	_ time.Time,
	leaseUntil time.Time,
	retryCount int,
) (int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, found := s.records[id]
	if !found || !token.ExpiresAt.After(time.Now().UTC()) ||
		(expectedGeneration >= 0 && token.DeliveryGeneration != expectedGeneration) {
		return 0, false, nil
	}
	token.DeliveryGeneration++
	token.RetryCount = retryCount
	token.NextRetryAt = &leaseUntil
	token.PendingStateCiphertext = pendingState
	token.UpdatedAt = time.Now().UTC()
	s.records[id] = token
	return token.DeliveryGeneration, true, nil
}

func (s *liveActivityStoreStub) MarkLiveActivityPushRetry(
	_ context.Context,
	id string,
	generation int64,
	count int,
	next time.Time,
	pendingState string,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, found := s.records[id]
	if !found || token.DeliveryGeneration != generation {
		return nil
	}
	token.RetryCount = count
	token.NextRetryAt = &next
	token.PendingStateCiphertext = pendingState
	token.UpdatedAt = time.Now().UTC()
	s.records[id] = token
	return nil
}

func (s *liveActivityStoreStub) ClearLiveActivityPushRetry(
	_ context.Context,
	id string,
	generation int64,
	deliveredIncidentUpdatedAt time.Time,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, found := s.records[id]
	if !found || token.DeliveryGeneration != generation {
		return nil
	}
	token.RetryCount = 0
	token.NextRetryAt = nil
	token.PendingStateCiphertext = ""
	token.LastDeliveredIncidentUpdatedAt = &deliveredIncidentUpdatedAt
	token.UpdatedAt = time.Now().UTC()
	s.records[id] = token
	return nil
}

func (s *liveActivityStoreStub) DeleteExpiredLiveActivityPushTokens(_ context.Context, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, token := range s.records {
		if !token.ExpiresAt.After(now) {
			delete(s.records, id)
		}
	}
	return nil
}

func (s *liveActivityStoreStub) snapshot() []persistence.LiveActivityPushToken {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]persistence.LiveActivityPushToken, 0, len(s.records))
	for _, token := range s.records {
		result = append(result, token)
	}
	return result
}

func (s *liveActivityStoreStub) makeRetryDue(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token := s.records[id]
	due := time.Now().UTC().Add(-time.Second)
	token.NextRetryAt = &due
	s.records[id] = token
}

func (s *liveActivityStoreStub) markForReconciliation(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconcile[id] = true
}
