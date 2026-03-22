package webservice

import "strings"

// normalizeDockerImage strips tags/digests and canonicalizes common registry
// prefixes so mirrored registry paths still map to known services.
func normalizeDockerImage(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return ""
	}

	// Lowercase
	image = strings.ToLower(image)

	// Strip digest (@sha256:...)
	if idx := strings.Index(image, "@"); idx >= 0 {
		image = image[:idx]
	}

	// Strip tag — but only from the last segment to avoid stripping port from registry URLs.
	// Find the last '/' to isolate the image name portion.
	lastSlash := strings.LastIndex(image, "/")
	if lastSlash >= 0 {
		name := image[lastSlash+1:]
		if colonIdx := strings.LastIndex(name, ":"); colonIdx >= 0 {
			image = image[:lastSlash+1] + name[:colonIdx]
		}
	} else {
		// No slash — simple image name like "traefik:v3.0"
		if colonIdx := strings.LastIndex(image, ":"); colonIdx >= 0 {
			image = image[:colonIdx]
		}
	}

	// Strip common Docker Hub prefixes.
	image = strings.TrimPrefix(image, "registry-1.docker.io/library/")
	image = strings.TrimPrefix(image, "registry-1.docker.io/")
	image = strings.TrimPrefix(image, "index.docker.io/library/")
	image = strings.TrimPrefix(image, "index.docker.io/")
	image = strings.TrimPrefix(image, "docker.io/library/")
	image = strings.TrimPrefix(image, "docker.io/")

	// Strip registry hostname prefixes (for example ghcr.io/, lscr.io/,
	// localhost:5000/) while preserving organization/name segments.
	if slash := strings.Index(image, "/"); slash > 0 {
		hostPrefix := image[:slash]
		if hostPrefix == "localhost" || strings.Contains(hostPrefix, ".") || strings.Contains(hostPrefix, ":") {
			image = image[slash+1:]
		}
	}

	// Normalize Docker Hub library aliases.
	image = strings.TrimPrefix(image, "library/")

	return strings.TrimSpace(image)
}

func addHintAlias(key, raw string) {
	normalized := normalizeServiceHint(raw)
	if normalized == "" {
		return
	}
	if _, exists := hintIndex[normalized]; exists {
		return
	}
	hintIndex[normalized] = key
}

func normalizeServiceHint(raw string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(trimmed))
	for _, r := range trimmed {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hintCandidates(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	candidates := []string{trimmed}

	// For URL-like strings, include host and first DNS label.
	if strings.Contains(trimmed, "://") {
		withoutScheme := trimmed[strings.Index(trimmed, "://")+3:]
		if slash := strings.IndexByte(withoutScheme, '/'); slash >= 0 {
			withoutScheme = withoutScheme[:slash]
		}
		host := withoutScheme
		if at := strings.LastIndexByte(host, '@'); at >= 0 && at+1 < len(host) {
			host = host[at+1:]
		}
		if colon := strings.LastIndexByte(host, ':'); colon > 0 {
			host = host[:colon]
		}
		host = strings.Trim(host, "[]")
		if host != "" {
			candidates = append(candidates, host)
			if firstDot := strings.IndexByte(host, '.'); firstDot > 0 {
				candidates = append(candidates, host[:firstDot])
			}
		}
	}

	// Include first DNS label for domains.
	if firstDot := strings.IndexByte(trimmed, '.'); firstDot > 0 {
		candidates = append(candidates, trimmed[:firstDot])
	}

	// Include tail segment for image/container path hints.
	if slash := strings.LastIndexByte(trimmed, '/'); slash >= 0 && slash+1 < len(trimmed) {
		candidates = append(candidates, trimmed[slash+1:])
	}

	// Include tokenized hints from separators.
	tokens := strings.FieldsFunc(trimmed, func(r rune) bool {
		switch r {
		case '/', '\\', ':', '.', '-', '_', ' ', '@':
			return true
		default:
			return false
		}
	})
	for _, token := range tokens {
		if strings.TrimSpace(token) != "" {
			candidates = append(candidates, token)
		}
	}

	return candidates
}
