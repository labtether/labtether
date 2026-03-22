package truenas

import (
	"github.com/labtether/labtether/internal/hubapi/shared"
	"fmt"
	"strings"
	"time"

	tnconnector "github.com/labtether/labtether/internal/connectors/truenas"
	"github.com/labtether/labtether/internal/hubcollector"
)

func (d *Deps) IngestTrueNASSubscriptionEvent(collector hubcollector.Collector, collectorID string, event tnconnector.SubscriptionEvent) {
	fields := NormalizeTrueNASSubscriptionFields(event.Fields)
	logFields := map[string]string{
		"collector_id":   strings.TrimSpace(collectorID),
		"collector_type": hubcollector.CollectorTypeTrueNAS,
		"subscription":   strings.TrimSpace(event.Collection),
		"event_type":     strings.TrimSpace(event.MessageType),
	}
	CopySubscriptionField(logFields, fields, "hostname")
	CopySubscriptionField(logFields, fields, "uuid")
	CopySubscriptionField(logFields, fields, "klass")
	CopySubscriptionField(logFields, fields, "source")

	assetID := strings.TrimSpace(collector.AssetID)
	if hostname := strings.TrimSpace(fields["hostname"]); hostname != "" {
		assetID = "truenas-host-" + shared.NormalizeAssetKey(hostname)
	}

	timestamp := event.ReceivedAt.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	if datetime := strings.TrimSpace(fields["datetime"]); datetime != "" {
		timestamp = shared.CollectorAnyTime(datetime)
	}

	message := TrueNASSubscriptionMessage(event, fields)

	level := shared.NormalizeCollectorLogLevel(fields["level"])
	if level == "info" {
		switch strings.ToLower(strings.TrimSpace(event.MessageType)) {
		case "removed":
			level = "warn"
		case "failed", "error":
			level = "error"
		}
	}

	key := strings.TrimSpace(fields["uuid"])
	if key == "" {
		key = strings.TrimSpace(fields["id"])
	}
	if key == "" {
		key = strings.TrimSpace(event.EventID)
	}
	if key == "" {
		key = strings.TrimSpace(event.Collection) + "|" + strings.TrimSpace(event.MessageType) + "|" + timestamp.UTC().Format(time.RFC3339Nano)
	}

	d.AppendConnectorLogEventWithID(
		shared.StableConnectorLogID("log_truenas_sub", key),
		assetID,
		"truenas",
		level,
		message,
		logFields,
		timestamp,
	)
}

func NormalizeTrueNASSubscriptionFields(fields map[string]any) map[string]string {
	if len(fields) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(fields))
	for key, value := range fields {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		text := strings.TrimSpace(shared.CollectorAnyString(value))
		if text == "" {
			continue
		}
		out[trimmedKey] = text
	}
	return out
}

func CopySubscriptionField(target map[string]string, source map[string]string, key string) {
	if target == nil || source == nil {
		return
	}
	value := strings.TrimSpace(source[key])
	if value == "" {
		return
	}
	target[key] = value
}

func TrueNASSubscriptionMessage(event tnconnector.SubscriptionEvent, fields map[string]string) string {
	if strings.Contains(strings.ToLower(strings.TrimSpace(event.Collection)), "alert") {
		asAny := make(map[string]any, len(fields))
		for key, value := range fields {
			asAny[key] = value
		}
		return shared.TrueNASAlertMessage(asAny)
	}

	if message := strings.TrimSpace(fields["message"]); message != "" {
		return message
	}
	if message := strings.TrimSpace(fields["name"]); message != "" {
		return "truenas event: " + message
	}

	eventType := strings.TrimSpace(event.MessageType)
	if eventType == "" {
		eventType = "event"
	}
	collection := strings.TrimSpace(event.Collection)
	if collection == "" {
		collection = "subscription"
	}
	return fmt.Sprintf("truenas %s %s", collection, eventType)
}
