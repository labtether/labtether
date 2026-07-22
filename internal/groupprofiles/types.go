package groupprofiles

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
)

const (
	DriftStatusCompliant = "compliant"
	DriftStatusDrifted   = "drifted"
)

const (
	MaxExpectedAssetCount = 1_000_000
	MaxRequiredPlatforms  = 32
	MaxPlatformNameLength = 64
)

// NormalizeConfig defines the complete supported group-profile schema. This
// keeps the generic JSON field from becoming an unbounded or secret-bearing
// storage surface.
func NormalizeConfig(config map[string]any) (map[string]any, error) {
	if config == nil {
		return nil, errors.New("config is required")
	}
	normalized := make(map[string]any, len(config))
	for key, value := range config {
		switch key {
		case "expected_asset_count":
			number, ok := numericValue(value)
			if !ok || number != math.Trunc(number) || number < 0 || number > MaxExpectedAssetCount {
				return nil, fmt.Errorf("expected_asset_count must be an integer from 0 to %d", MaxExpectedAssetCount)
			}
			normalized[key] = int(number)
		case "min_online_percent":
			number, ok := numericValue(value)
			if !ok || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 || number > 100 {
				return nil, errors.New("min_online_percent must be a number from 0 to 100")
			}
			normalized[key] = number
		case "required_platforms":
			platforms, err := normalizePlatforms(value)
			if err != nil {
				return nil, err
			}
			normalized[key] = platforms
		default:
			return nil, fmt.Errorf("unsupported group profile config key: %s", key)
		}
	}
	return normalized, nil
}

// SanitizeLegacyConfig removes unsupported or invalid legacy values before a
// profile is returned to callers or consumed by drift evaluation.
func SanitizeLegacyConfig(config map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range config {
		normalized, err := NormalizeConfig(map[string]any{key: value})
		if err == nil {
			out[key] = normalized[key]
		}
	}
	return out
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	default:
		return 0, false
	}
}

func normalizePlatforms(value any) ([]string, error) {
	var input []string
	switch typed := value.(type) {
	case []string:
		input = typed
	case []any:
		input = make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, errors.New("required_platforms must contain only strings")
			}
			input = append(input, text)
		}
	default:
		return nil, errors.New("required_platforms must be an array of strings")
	}
	if len(input) > MaxRequiredPlatforms {
		return nil, fmt.Errorf("required_platforms exceeds %d entries", MaxRequiredPlatforms)
	}
	out := make([]string, 0, len(input))
	seen := map[string]struct{}{}
	for _, raw := range input {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" || len(value) > MaxPlatformNameLength {
			return nil, fmt.Errorf("required platform names must be 1-%d bytes", MaxPlatformNameLength)
		}
		for _, r := range value {
			if unicode.IsControl(r) {
				return nil, errors.New("required platform names cannot contain control characters")
			}
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out, nil
}

var (
	ErrProfileNotFound = errors.New("group profile not found")
)

type Profile struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Config      map[string]any `json:"config"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type CreateProfileRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Config      map[string]any `json:"config"`
}

type UpdateProfileRequest struct {
	Name        *string         `json:"name,omitempty"`
	Description *string         `json:"description,omitempty"`
	Config      *map[string]any `json:"config,omitempty"`
}

type Assignment struct {
	ID         string    `json:"id"`
	GroupID    string    `json:"group_id"`
	ProfileID  string    `json:"profile_id"`
	AssignedBy string    `json:"assigned_by,omitempty"`
	AssignedAt time.Time `json:"assigned_at"`
}

type DriftCheck struct {
	ID           string         `json:"id"`
	GroupID      string         `json:"group_id"`
	ProfileID    string         `json:"profile_id"`
	Status       string         `json:"status"`
	DriftDetails map[string]any `json:"drift_details,omitempty"`
	CheckedAt    time.Time      `json:"checked_at"`
}

func NormalizeDriftStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case DriftStatusCompliant:
		return DriftStatusCompliant
	case DriftStatusDrifted:
		return DriftStatusDrifted
	default:
		return ""
	}
}
