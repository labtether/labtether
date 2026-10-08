package runtimesettings

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func NormalizeValue(def Definition, raw string) (string, error) {
	value := raw
	if !def.PreserveWhitespace {
		value = strings.TrimSpace(value)
	}
	if def.MaxBytes > 0 && len(value) > def.MaxBytes {
		return "", fmt.Errorf("%s must be at most %d bytes", def.Key, def.MaxBytes)
	}
	switch def.Type {
	case ValueTypeString:
		if value == "" && !def.AllowEmpty {
			return "", fmt.Errorf("%s cannot be empty", def.Key)
		}
		return value, nil
	case ValueTypeBool:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return "", fmt.Errorf("%s must be true or false", def.Key)
		}
		return strconv.FormatBool(parsed), nil
	case ValueTypeInt:
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return "", fmt.Errorf("%s must be a number", def.Key)
		}
		if def.MinInt > 0 && parsed < def.MinInt {
			return "", fmt.Errorf("%s must be >= %d", def.Key, def.MinInt)
		}
		if def.MaxInt > 0 && parsed > def.MaxInt {
			return "", fmt.Errorf("%s must be <= %d", def.Key, def.MaxInt)
		}
		return strconv.Itoa(parsed), nil
	case ValueTypeDuration:
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			return "", fmt.Errorf("%s must be a positive duration", def.Key)
		}
		if def.MinDuration > 0 && parsed < def.MinDuration {
			return "", fmt.Errorf("%s must be >= %s", def.Key, def.MinDuration)
		}
		if def.MaxDuration > 0 && parsed > def.MaxDuration {
			return "", fmt.Errorf("%s must be <= %s", def.Key, def.MaxDuration)
		}
		return parsed.String(), nil
	case ValueTypeEnum:
		for _, allowed := range def.AllowedValues {
			if strings.EqualFold(value, allowed) {
				return allowed, nil
			}
		}
		return "", fmt.Errorf("%s must be one of: %s", def.Key, strings.Join(def.AllowedValues, ", "))
	case ValueTypeURL:
		if value == "" && def.AllowEmpty {
			return "", nil
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return "", fmt.Errorf("%s must be an absolute URL", def.Key)
		}
		return strings.TrimRight(parsed.String(), "/"), nil
	default:
		return "", fmt.Errorf("unsupported runtime setting type for %s", def.Key)
	}
}

func ResolveEnvValue(def Definition, lookup func(string) string) string {
	if lookup == nil || strings.TrimSpace(def.EnvVar) == "" {
		return ""
	}
	raw := lookup(def.EnvVar)
	if raw == "" || (!def.PreserveWhitespace && strings.TrimSpace(raw) == "") {
		return ""
	}
	normalized, err := NormalizeValue(def, raw)
	if err != nil {
		return ""
	}
	return normalized
}

func EffectiveValue(def Definition, envValue, overrideValue string) (string, Source) {
	if overrideValue != "" && (def.PreserveWhitespace || strings.TrimSpace(overrideValue) != "") {
		return overrideValue, SourceUI
	}
	if envValue != "" && (def.PreserveWhitespace || strings.TrimSpace(envValue) != "") {
		return envValue, SourceDocker
	}
	return def.DefaultValue, SourceDefault
}
