package collectors

import (
	"net/url"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
)

func (rule WebServiceAliasRule) apply(host string) (string, bool) {
	labels := strings.Split(strings.ToLower(strings.TrimSpace(host)), ".")
	if len(labels) != len(rule.fromLabels) {
		return "", false
	}

	captures := make([]string, 0, len(labels))
	for idx, pattern := range rule.fromLabels {
		if pattern == "*" {
			captures = append(captures, labels[idx])
			continue
		}
		if labels[idx] != pattern {
			return "", false
		}
	}

	out := make([]string, 0, len(rule.toLabels))
	captureIdx := 0
	for _, pattern := range rule.toLabels {
		if pattern == "*" {
			if captureIdx >= len(captures) {
				return "", false
			}
			out = append(out, captures[captureIdx])
			captureIdx++
			continue
		}
		out = append(out, pattern)
	}

	return strings.Join(out, "."), true
}

func canonicalWebServiceURLIdentity(raw string, aliasRules []WebServiceAliasRule) string {
	parsed := parseURLLoose(raw)
	if parsed == nil {
		return ""
	}

	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return ""
	}
	if len(aliasRules) > 0 {
		host = applyAliasRules(host, aliasRules)
	}

	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme == "" {
		scheme = "http"
	}

	port := strings.TrimSpace(parsed.Port())
	if port == "" {
		switch scheme {
		case "https":
			port = "443"
		default:
			port = "80"
		}
	}

	path := strings.TrimSpace(parsed.EscapedPath())
	if path == "" {
		path = "/"
	}

	return scheme + "://" + host + ":" + port + path
}

func webServiceURLIdentities(svc agentmgr.DiscoveredWebService) []string {
	return webServiceURLIdentitiesWithAliasRules(svc, nil)
}

func webServiceURLIdentitiesWithAliasRules(svc agentmgr.DiscoveredWebService, aliasRules []WebServiceAliasRule) []string {
	seen := make(map[string]struct{}, 4)
	out := make([]string, 0, 4)
	appendIdentity := func(raw string) {
		identity := canonicalWebServiceURLIdentity(raw, aliasRules)
		if identity == "" {
			return
		}
		if _, exists := seen[identity]; exists {
			return
		}
		seen[identity] = struct{}{}
		out = append(out, identity)
	}

	appendIdentity(svc.URL)
	if svc.Metadata != nil {
		for _, alias := range splitAliasCSV(svc.Metadata["alt_urls"]) {
			appendIdentity(alias)
		}
	}

	return out
}

func webServiceGroupingPairKey(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return ""
	}
	if strings.EqualFold(left, right) {
		return ""
	}
	if strings.Compare(left, right) > 0 {
		left, right = right, left
	}
	return left + "||" + right
}

func parseURLLoose(raw string) *url.URL {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	parsed, err := url.Parse(trimmed)
	if err == nil && strings.TrimSpace(parsed.Hostname()) != "" {
		return parsed
	}
	if strings.Contains(trimmed, "://") {
		return nil
	}
	parsed, err = url.Parse("http://" + trimmed)
	if err != nil || strings.TrimSpace(parsed.Hostname()) == "" {
		return nil
	}
	return parsed
}

func applyAliasRules(host string, rules []WebServiceAliasRule) string {
	normalized := strings.ToLower(strings.TrimSpace(host))
	for _, rule := range rules {
		if next, ok := rule.apply(normalized); ok {
			return next
		}
	}
	return normalized
}

func sharesAnyURLIdentity(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := seen[value]; ok {
			return true
		}
	}
	return false
}
