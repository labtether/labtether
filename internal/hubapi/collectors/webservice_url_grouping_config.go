package collectors

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"
)

func (d *Deps) ResolveWebServiceURLGroupingConfig() WebServiceURLGroupingConfig {
	now := time.Now().UTC()
	ttl := d.WebServiceURLGroupingCfgTTL
	if ttl <= 0 {
		ttl = defaultWebServiceURLGroupingCacheTTL
	}

	d.WebServiceURLGroupingCfgMu.RLock()
	if !d.WebServiceURLGroupingCfgAt.IsZero() && now.Sub(d.WebServiceURLGroupingCfgAt) < ttl {
		cached := d.WebServiceURLGroupingCfg
		d.WebServiceURLGroupingCfgMu.RUnlock()
		return cached
	}
	d.WebServiceURLGroupingCfgMu.RUnlock()

	cfg := d.resolveWebServiceURLGroupingConfigUncached()

	d.WebServiceURLGroupingCfgMu.Lock()
	d.WebServiceURLGroupingCfg = cfg
	d.WebServiceURLGroupingCfgAt = now
	d.WebServiceURLGroupingCfgMu.Unlock()

	return cfg
}

func (d *Deps) resolveWebServiceURLGroupingConfigUncached() WebServiceURLGroupingConfig {
	cfg := WebServiceURLGroupingConfig{
		Mode:                WebServiceURLGroupingModeConservative,
		DryRun:              false,
		ConfidenceThreshold: 85,
		AliasRules:          nil,
		ForceGroupRules:     nil,
		NeverGroupRules:     nil,
	}

	// Try loading settings from the dedicated grouping settings table first.
	if d.DB != nil {
		ctx := context.Background()
		dbSettings, err := d.DB.ListURLGroupingSettings(ctx)
		if err != nil {
			log.Printf("webservices: failed to load url grouping settings from db: %v", err)
		} else if len(dbSettings) > 0 {
			settingsMap := make(map[string]string, len(dbSettings))
			for _, row := range dbSettings {
				settingsMap[row.SettingKey] = row.SettingValue
			}
			cfg.Mode = parseWebServiceURLGroupingMode(settingsMap["grouping_mode"])
			cfg.DryRun = parseBoolSettingOrDefault(settingsMap["dry_run"], false)
			cfg.ConfidenceThreshold = parseIntSettingOrDefault(settingsMap["sensitivity"], 85, 0, 100)
			cfg.AliasRules = ParseWebServiceAliasRules(settingsMap["alias_rules"])
			cfg.ForceGroupRules = ParseWebServicePairRules(settingsMap["force_group_rules"])

			// Load never-group rules from the dedicated table.
			neverGroupRules, ngErr := d.DB.ListNeverGroupRules(ctx)
			if ngErr != nil {
				log.Printf("webservices: failed to load never-group rules from db: %v", ngErr)
			} else {
				neverGroup := make(map[string]struct{}, len(neverGroupRules))
				for _, rule := range neverGroupRules {
					key := webServiceGroupingPairKey(
						canonicalWebServiceURLIdentity(rule.URLA, nil),
						canonicalWebServiceURLIdentity(rule.URLB, nil),
					)
					if key != "" {
						neverGroup[key] = struct{}{}
					}
				}
				if len(neverGroup) > 0 {
					cfg.NeverGroupRules = neverGroup
				}
			}
			return cfg
		}
	}

	return cfg
}

func (d *Deps) InvalidateWebServiceURLGroupingConfigCache() {
	d.WebServiceURLGroupingCfgMu.Lock()
	d.WebServiceURLGroupingCfg = WebServiceURLGroupingConfig{}
	d.WebServiceURLGroupingCfgAt = time.Time{}
	d.WebServiceURLGroupingCfgMu.Unlock()
}

func parseWebServiceURLGroupingMode(raw string) WebServiceURLGroupingMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(WebServiceURLGroupingModeOff):
		return WebServiceURLGroupingModeOff
	case string(WebServiceURLGroupingModeBalanced):
		return WebServiceURLGroupingModeBalanced
	case string(WebServiceURLGroupingModeAggressive):
		return WebServiceURLGroupingModeAggressive
	default:
		return WebServiceURLGroupingModeConservative
	}
}

func parseBoolSettingOrDefault(raw string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true":
		return true
	case "false":
		return false
	default:
		return fallback
	}
}

func parseIntSettingOrDefault(raw string, fallback, min, max int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	if parsed < min {
		return min
	}
	if parsed > max {
		return max
	}
	return parsed
}

func ParseWebServiceAliasRules(raw string) []WebServiceAliasRule {
	lines := splitRuleLines(raw)
	if len(lines) == 0 {
		return nil
	}

	out := make([]WebServiceAliasRule, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "=>")
		if len(parts) != 2 {
			continue
		}

		from := normalizeDomainPattern(parts[0])
		to := normalizeDomainPattern(parts[1])
		if from == "" || to == "" {
			continue
		}

		fromLabels := strings.Split(from, ".")
		toLabels := strings.Split(to, ".")
		if len(fromLabels) == 0 || len(toLabels) == 0 {
			continue
		}
		if !isValidDomainPattern(fromLabels) || !isValidDomainPattern(toLabels) {
			continue
		}

		fromWildcards := 0
		for _, label := range fromLabels {
			if label == "*" {
				fromWildcards++
			}
		}
		toWildcards := 0
		for _, label := range toLabels {
			if label == "*" {
				toWildcards++
			}
		}
		if toWildcards > fromWildcards {
			continue
		}

		out = append(out, WebServiceAliasRule{
			fromLabels: fromLabels,
			toLabels:   toLabels,
		})
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func ParseWebServicePairRules(raw string) map[string]struct{} {
	lines := splitRuleLines(raw)
	if len(lines) == 0 {
		return nil
	}

	out := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		leftRaw, rightRaw, ok := splitWebServicePairRuleLine(line)
		if !ok {
			continue
		}
		leftID := canonicalWebServiceURLIdentity(leftRaw, nil)
		rightID := canonicalWebServiceURLIdentity(rightRaw, nil)
		key := webServiceGroupingPairKey(leftID, rightID)
		if key == "" {
			continue
		}
		out[key] = struct{}{}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func splitWebServicePairRuleLine(line string) (string, string, bool) {
	separators := []string{"<=>", "=>", ","}
	for _, separator := range separators {
		if !strings.Contains(line, separator) {
			continue
		}
		parts := strings.Split(line, separator)
		if len(parts) != 2 {
			return "", "", false
		}
		left := strings.TrimSpace(parts[0])
		right := strings.TrimSpace(parts[1])
		if left == "" || right == "" {
			return "", "", false
		}
		return left, right, true
	}
	return "", "", false
}

func splitRuleLines(raw string) []string {
	chunks := strings.Split(raw, "\n")
	out := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		line := strings.TrimSpace(chunk)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func normalizeDomainPattern(raw string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(raw)), ".")
}

func isValidDomainPattern(labels []string) bool {
	for _, label := range labels {
		if label == "" {
			return false
		}
		if label == "*" {
			continue
		}
		for _, ch := range label {
			isLetter := ch >= 'a' && ch <= 'z'
			isNumber := ch >= '0' && ch <= '9'
			if !isLetter && !isNumber && ch != '-' {
				return false
			}
		}
	}
	return true
}
