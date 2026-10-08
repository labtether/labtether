package alerting

import (
	"context"
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/notifications"
	"reflect"
	"sync"
	"time"
)

type notificationSecurityStore struct {
	mu       sync.Mutex
	channels map[string]notifications.Channel
	records  []notifications.Record
	casCount int
	casRace  map[string]any
}

func newNotificationSecurityStore() *notificationSecurityStore {
	return &notificationSecurityStore{channels: make(map[string]notifications.Channel)}
}

func (s *notificationSecurityStore) CreateNotificationChannel(req notifications.CreateChannelRequest) (notifications.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	channel := notifications.Channel{
		ID:        req.ID,
		Name:      req.Name,
		Type:      req.Type,
		Config:    deepCloneNotificationMap(req.Config),
		Enabled:   enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.channels[channel.ID] = channel
	return cloneNotificationSecurityChannel(channel), nil
}

func (s *notificationSecurityStore) GetNotificationChannel(id string) (notifications.Channel, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	channel, ok := s.channels[id]
	return cloneNotificationSecurityChannel(channel), ok, nil
}

func (s *notificationSecurityStore) ListNotificationChannels(_ int) ([]notifications.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	channels := make([]notifications.Channel, 0, len(s.channels))
	for _, channel := range s.channels {
		channels = append(channels, cloneNotificationSecurityChannel(channel))
	}
	return channels, nil
}

func (s *notificationSecurityStore) UpdateNotificationChannel(id string, req notifications.UpdateChannelRequest) (notifications.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	channel, ok := s.channels[id]
	if !ok {
		return notifications.Channel{}, notifications.ErrChannelNotFound
	}
	if req.Name != nil {
		channel.Name = *req.Name
	}
	if req.Config != nil {
		channel.Config = deepCloneNotificationMap(*req.Config)
	}
	if req.Enabled != nil {
		channel.Enabled = *req.Enabled
	}
	channel.UpdatedAt = time.Now().UTC()
	s.channels[id] = channel
	return cloneNotificationSecurityChannel(channel), nil
}

func (s *notificationSecurityStore) DeleteNotificationChannel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.channels[id]; !ok {
		return notifications.ErrChannelNotFound
	}
	delete(s.channels, id)
	return nil
}

func (s *notificationSecurityStore) CompareAndSwapNotificationChannelConfig(id string, expected, replacement map[string]any) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	channel, ok := s.channels[id]
	if !ok {
		return false, nil
	}
	if s.casRace != nil {
		channel.Config = deepCloneNotificationMap(s.casRace)
		s.channels[id] = channel
		s.casRace = nil
		return false, nil
	}
	if !reflect.DeepEqual(channel.Config, expected) {
		return false, nil
	}
	channel.Config = deepCloneNotificationMap(replacement)
	channel.UpdatedAt = time.Now().UTC()
	s.channels[id] = channel
	s.casCount++
	return true, nil
}

func (s *notificationSecurityStore) CreateAlertRoute(notifications.CreateRouteRequest) (notifications.Route, error) {
	return notifications.Route{}, errors.New("unused notification route store method")
}

func (s *notificationSecurityStore) GetAlertRoute(string) (notifications.Route, bool, error) {
	return notifications.Route{}, false, errors.New("unused notification route store method")
}

func (s *notificationSecurityStore) ListAlertRoutes(int) ([]notifications.Route, error) {
	return nil, errors.New("unused notification route store method")
}

func (s *notificationSecurityStore) UpdateAlertRoute(string, notifications.UpdateRouteRequest) (notifications.Route, error) {
	return notifications.Route{}, errors.New("unused notification route store method")
}

func (s *notificationSecurityStore) DeleteAlertRoute(string) error {
	return errors.New("unused notification route store method")
}

func (s *notificationSecurityStore) CreateNotificationRecord(req notifications.CreateRecordRequest) (notifications.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := notifications.Record{
		ID:              fmt.Sprintf("record-%d", len(s.records)+1),
		ChannelID:       req.ChannelID,
		AlertInstanceID: req.AlertInstanceID,
		RouteID:         req.RouteID,
		Payload:         deepCloneNotificationMap(req.Payload),
		Status:          req.Status,
		Error:           req.Error,
		MaxRetries:      notifications.DefaultMaxRetries,
		CreatedAt:       time.Now().UTC(),
	}
	s.records = append(s.records, record)
	return record, nil
}

func (s *notificationSecurityStore) ListNotificationHistory(_ int, channelID string) ([]notifications.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records := make([]notifications.Record, 0, len(s.records))
	for _, record := range s.records {
		if channelID == "" || record.ChannelID == channelID {
			records = append(records, record)
		}
	}
	return records, nil
}

func (s *notificationSecurityStore) ListPendingRetries(context.Context, time.Time, int) ([]notifications.Record, error) {
	return nil, nil
}

func (s *notificationSecurityStore) UpdateRetryState(_ context.Context, id string, retryCount int, nextRetryAt *time.Time, status, errorMessage string, payload map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.records {
		if s.records[i].ID == id {
			s.records[i].RetryCount = retryCount
			s.records[i].NextRetryAt = nextRetryAt
			s.records[i].Status = status
			s.records[i].Error = errorMessage
			if payload != nil {
				s.records[i].Payload = deepCloneNotificationMap(payload)
			}
		}
	}
	return nil
}

func (s *notificationSecurityStore) seed(channel notifications.Channel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channels[channel.ID] = cloneNotificationSecurityChannel(channel)
}

func (s *notificationSecurityStore) replaceConfig(id string, config map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	channel := s.channels[id]
	channel.Config = deepCloneNotificationMap(config)
	s.channels[id] = channel
}

func (s *notificationSecurityStore) migrations() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.casCount
}

func (s *notificationSecurityStore) forceCASRace(config map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.casRace = deepCloneNotificationMap(config)
}

func cloneNotificationSecurityChannel(channel notifications.Channel) notifications.Channel {
	channel.Config = deepCloneNotificationMap(channel.Config)
	return channel
}
