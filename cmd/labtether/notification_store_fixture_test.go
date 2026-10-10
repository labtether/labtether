package main

import (
	"context"
	"github.com/labtether/labtether/internal/notifications"
	"sort"
	"strings"
	"time"
)

type notificationStoreStub struct {
	channels      map[string]notifications.Channel
	routes        map[string]notifications.Route
	records       []notifications.Record
	retryUpdates  []notificationRetryUpdate
	nextRecordNum int
}

type notificationRetryUpdate struct {
	ID          string
	RetryCount  int
	NextRetryAt *time.Time
	Status      string
	Error       string
}

func newNotificationStoreStub() *notificationStoreStub {
	return &notificationStoreStub{
		channels:      make(map[string]notifications.Channel),
		routes:        make(map[string]notifications.Route),
		records:       make([]notifications.Record, 0, 8),
		retryUpdates:  make([]notificationRetryUpdate, 0, 8),
		nextRecordNum: 1,
	}
}

func (s *notificationStoreStub) CreateNotificationChannel(req notifications.CreateChannelRequest) (notifications.Channel, error) {
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = "nch-" + req.Name
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	ch := notifications.Channel{
		ID:        id,
		Name:      req.Name,
		Type:      req.Type,
		Config:    cloneAnyMap(req.Config),
		Enabled:   enabled,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	s.channels[id] = ch
	return ch, nil
}

func (s *notificationStoreStub) GetNotificationChannel(id string) (notifications.Channel, bool, error) {
	ch, ok := s.channels[id]
	return ch, ok, nil
}

func (s *notificationStoreStub) ListNotificationChannels(limit, offset int) ([]notifications.Channel, error) {
	out := make([]notifications.Channel, 0, len(s.channels))
	for _, ch := range s.channels {
		out = append(out, ch)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if offset >= len(out) {
		return []notifications.Channel{}, nil
	}
	return out[offset:min(offset+limit, len(out))], nil
}

func (s *notificationStoreStub) UpdateNotificationChannel(id string, req notifications.UpdateChannelRequest) (notifications.Channel, error) {
	ch, ok := s.channels[id]
	if !ok {
		return notifications.Channel{}, notifications.ErrChannelNotFound
	}
	if req.Name != nil {
		ch.Name = *req.Name
	}
	if req.Config != nil {
		ch.Config = cloneAnyMap(*req.Config)
	}
	if req.Enabled != nil {
		ch.Enabled = *req.Enabled
	}
	ch.UpdatedAt = time.Now().UTC()
	s.channels[id] = ch
	return ch, nil
}

func (s *notificationStoreStub) DeleteNotificationChannel(id string) error {
	if _, ok := s.channels[id]; !ok {
		return notifications.ErrChannelNotFound
	}
	delete(s.channels, id)
	return nil
}

func (s *notificationStoreStub) CreateAlertRoute(req notifications.CreateRouteRequest) (notifications.Route, error) {
	id := "route-" + req.Name
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	route := notifications.Route{
		ID:                    id,
		Name:                  req.Name,
		Matchers:              cloneAnyMap(req.Matchers),
		ChannelIDs:            append([]string(nil), req.ChannelIDs...),
		SeverityFilter:        req.SeverityFilter,
		GroupFilter:           req.GroupFilter,
		GroupBy:               append([]string(nil), req.GroupBy...),
		GroupWaitSeconds:      req.GroupWaitSeconds,
		GroupIntervalSeconds:  req.GroupIntervalSeconds,
		RepeatIntervalSeconds: req.RepeatIntervalSeconds,
		Enabled:               enabled,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
	}
	s.routes[id] = route
	return route, nil
}

func (s *notificationStoreStub) GetAlertRoute(id string) (notifications.Route, bool, error) {
	route, ok := s.routes[id]
	return route, ok, nil
}

func (s *notificationStoreStub) ListAlertRoutes(_ int) ([]notifications.Route, error) {
	out := make([]notifications.Route, 0, len(s.routes))
	for _, route := range s.routes {
		out = append(out, route)
	}
	return out, nil
}

func (s *notificationStoreStub) UpdateAlertRoute(id string, req notifications.UpdateRouteRequest) (notifications.Route, error) {
	route, ok := s.routes[id]
	if !ok {
		return notifications.Route{}, notifications.ErrRouteNotFound
	}
	if req.Name != nil {
		route.Name = *req.Name
	}
	if req.Matchers != nil {
		route.Matchers = cloneAnyMap(*req.Matchers)
	}
	if req.ChannelIDs != nil {
		route.ChannelIDs = append([]string(nil), (*req.ChannelIDs)...)
	}
	if req.SeverityFilter != nil {
		route.SeverityFilter = *req.SeverityFilter
	}
	if req.GroupFilter != nil {
		route.GroupFilter = *req.GroupFilter
	}
	if req.GroupBy != nil {
		route.GroupBy = append([]string(nil), (*req.GroupBy)...)
	}
	if req.GroupWaitSeconds != nil {
		route.GroupWaitSeconds = *req.GroupWaitSeconds
	}
	if req.GroupIntervalSeconds != nil {
		route.GroupIntervalSeconds = *req.GroupIntervalSeconds
	}
	if req.RepeatIntervalSeconds != nil {
		route.RepeatIntervalSeconds = *req.RepeatIntervalSeconds
	}
	if req.Enabled != nil {
		route.Enabled = *req.Enabled
	}
	route.UpdatedAt = time.Now().UTC()
	s.routes[id] = route
	return route, nil
}

func (s *notificationStoreStub) DeleteAlertRoute(id string) error {
	if _, ok := s.routes[id]; !ok {
		return notifications.ErrRouteNotFound
	}
	delete(s.routes, id)
	return nil
}

func (s *notificationStoreStub) CreateNotificationRecord(req notifications.CreateRecordRequest) (notifications.Record, error) {
	now := time.Now().UTC()
	rec := notifications.Record{
		ID:              "notif-record-" + time.Now().UTC().Format("150405.000000000"),
		ChannelID:       req.ChannelID,
		AlertInstanceID: req.AlertInstanceID,
		RouteID:         req.RouteID,
		Payload:         cloneAnyMap(req.Payload),
		Status:          req.Status,
		Error:           req.Error,
		MaxRetries:      notifications.DefaultMaxRetries,
		CreatedAt:       now,
	}
	if rec.Status == notifications.RecordStatusSent {
		rec.SentAt = &now
	}
	s.nextRecordNum++
	s.records = append(s.records, rec)
	return rec, nil
}

func (s *notificationStoreStub) ListNotificationHistory(_ int, channelID string) ([]notifications.Record, error) {
	out := make([]notifications.Record, 0, len(s.records))
	for _, rec := range s.records {
		if channelID != "" && rec.ChannelID != channelID {
			continue
		}
		out = append(out, notifications.Record{
			ID:              rec.ID,
			ChannelID:       rec.ChannelID,
			AlertInstanceID: rec.AlertInstanceID,
			RouteID:         rec.RouteID,
			Payload:         cloneAnyMap(rec.Payload),
			Status:          rec.Status,
			SentAt:          rec.SentAt,
			Error:           rec.Error,
			RetryCount:      rec.RetryCount,
			MaxRetries:      rec.MaxRetries,
			NextRetryAt:     rec.NextRetryAt,
			CreatedAt:       rec.CreatedAt,
		})
	}
	return out, nil
}

func (s *notificationStoreStub) ListPendingRetries(_ context.Context, now time.Time, limit int) ([]notifications.Record, error) {
	if limit <= 0 {
		limit = 50
	}
	out := make([]notifications.Record, 0, limit)
	for _, rec := range s.records {
		if rec.Status != notifications.RecordStatusFailed || rec.NextRetryAt == nil {
			continue
		}
		if rec.RetryCount >= rec.MaxRetries {
			continue
		}
		if rec.NextRetryAt.After(now) {
			continue
		}
		out = append(out, notifications.Record{
			ID:              rec.ID,
			ChannelID:       rec.ChannelID,
			AlertInstanceID: rec.AlertInstanceID,
			RouteID:         rec.RouteID,
			Payload:         cloneAnyMap(rec.Payload),
			Status:          rec.Status,
			SentAt:          rec.SentAt,
			Error:           rec.Error,
			RetryCount:      rec.RetryCount,
			MaxRetries:      rec.MaxRetries,
			NextRetryAt:     rec.NextRetryAt,
			CreatedAt:       rec.CreatedAt,
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *notificationStoreStub) UpdateRetryState(_ context.Context, id string, retryCount int, nextRetryAt *time.Time, status, errorMessage string, payload map[string]any) error {
	s.retryUpdates = append(s.retryUpdates, notificationRetryUpdate{
		ID:          id,
		RetryCount:  retryCount,
		NextRetryAt: nextRetryAt,
		Status:      status,
		Error:       errorMessage,
	})
	for idx := range s.records {
		if s.records[idx].ID != id {
			continue
		}
		s.records[idx].RetryCount = retryCount
		s.records[idx].NextRetryAt = nextRetryAt
		s.records[idx].Status = status
		s.records[idx].Error = errorMessage
		if payload != nil {
			s.records[idx].Payload = cloneAnyMap(payload)
		}
		if status == notifications.RecordStatusSent {
			sentAt := time.Now().UTC()
			s.records[idx].SentAt = &sentAt
			s.records[idx].Error = ""
		} else {
			s.records[idx].SentAt = nil
		}
		break
	}
	return nil
}
