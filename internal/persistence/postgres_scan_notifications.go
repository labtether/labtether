package persistence

import (
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/notifications"
)

type alertInstanceScanner interface {
	Scan(dest ...any) error
}

func scanAlertInstance(row alertInstanceScanner) (alerts.AlertInstance, error) {
	inst := alerts.AlertInstance{}
	var labels []byte
	var annotations []byte
	var resolvedAt *time.Time
	var suppressedBy *string
	if err := row.Scan(
		&inst.ID,
		&inst.RuleID,
		&inst.Fingerprint,
		&inst.Status,
		&inst.Severity,
		&labels,
		&annotations,
		&inst.StartedAt,
		&resolvedAt,
		&inst.LastFiredAt,
		&suppressedBy,
		&inst.CreatedAt,
		&inst.UpdatedAt,
	); err != nil {
		return alerts.AlertInstance{}, err
	}
	inst.Labels = unmarshalStringMap(labels)
	inst.Annotations = unmarshalStringMap(annotations)
	if resolvedAt != nil {
		v := resolvedAt.UTC()
		inst.ResolvedAt = &v
	}
	if suppressedBy != nil {
		inst.SuppressedBy = *suppressedBy
	}
	inst.StartedAt = inst.StartedAt.UTC()
	inst.LastFiredAt = inst.LastFiredAt.UTC()
	inst.CreatedAt = inst.CreatedAt.UTC()
	inst.UpdatedAt = inst.UpdatedAt.UTC()
	return inst, nil
}

type alertSilenceScanner interface {
	Scan(dest ...any) error
}

func scanAlertSilence(row alertSilenceScanner) (alerts.AlertSilence, error) {
	silence := alerts.AlertSilence{}
	var matchers []byte
	if err := row.Scan(
		&silence.ID,
		&matchers,
		&silence.Reason,
		&silence.CreatedBy,
		&silence.StartsAt,
		&silence.EndsAt,
		&silence.CreatedAt,
	); err != nil {
		return alerts.AlertSilence{}, err
	}
	silence.Matchers = unmarshalStringMap(matchers)
	silence.StartsAt = silence.StartsAt.UTC()
	silence.EndsAt = silence.EndsAt.UTC()
	silence.CreatedAt = silence.CreatedAt.UTC()
	return silence, nil
}

type notificationChannelScanner interface {
	Scan(dest ...any) error
}

func scanNotificationChannel(row notificationChannelScanner) (notifications.Channel, error) {
	ch := notifications.Channel{}
	var config []byte
	if err := row.Scan(
		&ch.ID,
		&ch.Name,
		&ch.Type,
		&config,
		&ch.Enabled,
		&ch.CreatedAt,
		&ch.UpdatedAt,
	); err != nil {
		return notifications.Channel{}, err
	}
	ch.Config = unmarshalAnyMap(config)
	ch.CreatedAt = ch.CreatedAt.UTC()
	ch.UpdatedAt = ch.UpdatedAt.UTC()
	return ch, nil
}

type alertRouteScanner interface {
	Scan(dest ...any) error
}

func scanAlertRoute(row alertRouteScanner) (notifications.Route, error) {
	route := notifications.Route{}
	var matchers []byte
	var channelIDs []byte
	var severityFilter *string
	var groupFilter *string
	var groupBy []byte
	if err := row.Scan(
		&route.ID,
		&route.Name,
		&matchers,
		&channelIDs,
		&severityFilter,
		&groupFilter,
		&groupBy,
		&route.GroupWaitSeconds,
		&route.GroupIntervalSeconds,
		&route.RepeatIntervalSeconds,
		&route.Enabled,
		&route.CreatedAt,
		&route.UpdatedAt,
	); err != nil {
		return notifications.Route{}, err
	}
	route.Matchers = unmarshalAnyMap(matchers)
	route.ChannelIDs = unmarshalStringSlice(channelIDs)
	if severityFilter != nil {
		route.SeverityFilter = *severityFilter
	}
	if groupFilter != nil {
		route.GroupFilter = *groupFilter
	}
	// Grouping/repeat semantics are not implemented by the dispatcher yet, so
	// keep the API surface honest by normalizing any legacy stored values away.
	route.GroupBy = nil
	route.GroupWaitSeconds = 0
	route.GroupIntervalSeconds = 0
	route.RepeatIntervalSeconds = 0
	route.CreatedAt = route.CreatedAt.UTC()
	route.UpdatedAt = route.UpdatedAt.UTC()
	return route, nil
}

type notificationRecordScanner interface {
	Scan(dest ...any) error
}

func scanNotificationRecord(row notificationRecordScanner) (notifications.Record, error) {
	rec := notifications.Record{}
	var alertInstanceID *string
	var routeID *string
	var payload []byte
	var sentAt *time.Time
	var nextRetryAt *time.Time
	if err := row.Scan(
		&rec.ID,
		&rec.ChannelID,
		&alertInstanceID,
		&routeID,
		&payload,
		&rec.Status,
		&sentAt,
		&rec.Error,
		&rec.RetryCount,
		&rec.MaxRetries,
		&nextRetryAt,
		&rec.CreatedAt,
	); err != nil {
		return notifications.Record{}, err
	}
	if alertInstanceID != nil {
		rec.AlertInstanceID = *alertInstanceID
	}
	if routeID != nil {
		rec.RouteID = *routeID
	}
	rec.Payload = unmarshalAnyMap(payload)
	if sentAt != nil {
		v := sentAt.UTC()
		rec.SentAt = &v
	}
	if nextRetryAt != nil {
		v := nextRetryAt.UTC()
		rec.NextRetryAt = &v
	}
	rec.CreatedAt = rec.CreatedAt.UTC()
	return rec, nil
}
