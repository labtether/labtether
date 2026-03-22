package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/retention"
	"github.com/labtether/labtether/internal/updates"
)

func marshalStringMap(values map[string]string) (string, error) {
	if len(values) == 0 {
		return "{}", nil
	}

	payload, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func unmarshalStringMap(payload []byte) map[string]string {
	if len(payload) == 0 {
		return nil
	}

	values := make(map[string]string)
	if err := json.Unmarshal(payload, &values); err != nil {
		return nil
	}
	return cloneMetadata(values)
}

func marshalAnyMap(values map[string]any) (string, error) {
	if len(values) == 0 {
		return "{}", nil
	}

	payload, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func unmarshalAnyMap(payload []byte) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	values := make(map[string]any)
	if err := json.Unmarshal(payload, &values); err != nil {
		return nil
	}
	return cloneAnyMap(values)
}

func floatPtr(value float64) *float64 {
	out := value
	return &out
}

func nullIfBlank(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func nullFloat64(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC()
}

func marshalStringSlice(values []string) (string, error) {
	clean := sanitizeStringSlice(values)
	payload, err := json.Marshal(clean)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func unmarshalStringSlice(payload []byte) []string {
	if len(payload) == 0 {
		return nil
	}

	out := make([]string, 0, 8)
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil
	}
	return sanitizeStringSlice(out)
}

func marshalUpdateRunResults(values []updates.RunResultEntry) (string, error) {
	if len(values) == 0 {
		return "[]", nil
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func unmarshalUpdateRunResults(payload []byte) []updates.RunResultEntry {
	if len(payload) == 0 {
		return nil
	}

	out := make([]updates.RunResultEntry, 0, 16)
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil
	}
	return out
}

func sanitizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func execDeleteRows(tx pgx.Tx, query string, args ...any) (int64, error) {
	tag, err := tx.Exec(context.Background(), query, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func parseRetentionDuration(raw string, fallback time.Duration) time.Duration {
	parsed, err := retention.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func DefaultDatabaseURL(host string) string {
	return fmt.Sprintf("postgres://labtether:labtether@%s:5432/labtether?sslmode=disable", host)
}
