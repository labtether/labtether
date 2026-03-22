package collectors

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	webServiceCustomIconLibraryKey       = "services.custom_icon_library"
	maxCustomServiceIconLibraryItems     = 100
	maxCustomServiceIconDataURLLength    = 800 * 1024
	maxCustomServiceIconDisplayNameChars = 64
)

func (d *Deps) loadWebServiceIconLibrary() ([]WebServiceIconLibraryEntry, error) {
	if d.RuntimeStore == nil {
		return nil, errors.New("runtime settings unavailable")
	}
	overrides, err := d.RuntimeStore.ListRuntimeSettingOverrides()
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(overrides[webServiceCustomIconLibraryKey])
	if raw == "" {
		return []WebServiceIconLibraryEntry{}, nil
	}

	var parsed []WebServiceIconLibraryEntry
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, err
	}

	normalized := make([]WebServiceIconLibraryEntry, 0, len(parsed))
	seen := make(map[string]struct{}, len(parsed))
	for _, entry := range parsed {
		id := strings.TrimSpace(entry.ID)
		name := normalizeCustomServiceIconDisplayName(entry.Name)
		dataURL := strings.TrimSpace(entry.DataURL)
		if id == "" || name == "" || dataURL == "" {
			continue
		}
		if len(name) > maxCustomServiceIconDisplayNameChars {
			name = name[:maxCustomServiceIconDisplayNameChars]
		}
		if err := validateCustomServiceIconDataURL(dataURL); err != nil {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, WebServiceIconLibraryEntry{
			ID:        id,
			Name:      name,
			DataURL:   dataURL,
			CreatedAt: strings.TrimSpace(entry.CreatedAt),
			UpdatedAt: strings.TrimSpace(entry.UpdatedAt),
		})
	}
	return normalized, nil
}

func (d *Deps) saveWebServiceIconLibrary(icons []WebServiceIconLibraryEntry) error {
	if d.RuntimeStore == nil {
		return errors.New("runtime settings unavailable")
	}
	if len(icons) == 0 {
		return d.RuntimeStore.DeleteRuntimeSettingOverrides([]string{webServiceCustomIconLibraryKey})
	}
	encoded, err := json.Marshal(icons)
	if err != nil {
		return err
	}
	_, err = d.RuntimeStore.SaveRuntimeSettingOverrides(map[string]string{
		webServiceCustomIconLibraryKey: string(encoded),
	})
	return err
}

func validateCustomServiceIconDataURL(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return errors.New("data_url is required")
	}
	lower := strings.ToLower(value)
	if len(value) > maxCustomServiceIconDataURLLength {
		return errors.New("data_url is too large")
	}

	allowedPrefixes := []string{
		"data:image/png;base64,",
		"data:image/jpeg;base64,",
		"data:image/webp;base64,",
		"data:image/gif;base64,",
		"data:image/svg+xml;base64,",
	}
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(lower, prefix) && len(value) > len(prefix) {
			return nil
		}
	}
	return errors.New("data_url must be a base64 image data URL")
}

func normalizeCustomServiceIconDisplayName(raw string) string {
	name := strings.TrimSpace(raw)
	if name == "" {
		return ""
	}
	return strings.Join(strings.Fields(name), " ")
}

func generateCustomServiceIconID(name string) string {
	base := normalizeCustomServiceIconSlug(name)
	if base == "" {
		base = "icon"
	}
	return base + "-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
}

func normalizeCustomServiceIconSlug(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	if lower == "" {
		return ""
	}
	var out strings.Builder
	out.Grow(len(lower))
	lastDash := false
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
			lastDash = false
			continue
		}
		if lastDash {
			continue
		}
		out.WriteByte('-')
		lastDash = true
	}
	return strings.Trim(out.String(), "-")
}
