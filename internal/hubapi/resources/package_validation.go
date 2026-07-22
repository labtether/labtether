package resources

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	maxPackageActionCount     = 128
	maxPackageActionNameBytes = 128
)

func normalizeAndValidatePackageTokens(packages []string) ([]string, error) {
	if len(packages) > maxPackageActionCount {
		return nil, fmt.Errorf("package list exceeds the maximum of %d entries", maxPackageActionCount)
	}

	seen := make(map[string]struct{}, len(packages))
	normalized := make([]string, 0, len(packages))
	for index, raw := range packages {
		if strings.IndexFunc(raw, unicode.IsControl) >= 0 {
			return nil, fmt.Errorf("package entry %d contains a control character", index+1)
		}
		name := strings.TrimSpace(raw)
		if name == "" {
			return nil, fmt.Errorf("package entry %d is empty", index+1)
		}
		if len(name) > maxPackageActionNameBytes {
			return nil, fmt.Errorf("package entry %d exceeds %d bytes", index+1, maxPackageActionNameBytes)
		}
		if name[0] == '-' {
			return nil, fmt.Errorf("package %q must not begin with a hyphen", name)
		}
		if !validPackageActionName(name) {
			return nil, fmt.Errorf("package %q includes unsupported characters or path segments", name)
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		normalized = append(normalized, name)
	}
	return normalized, nil
}

func validPackageActionName(name string) bool {
	for index := 0; index < len(name); index++ {
		char := name[index]
		if index == 0 {
			if !isPackageASCIIAlphaNumeric(char) && char != '@' {
				return false
			}
			continue
		}
		if !isPackageASCIIAlphaNumeric(char) && !strings.ContainsRune("@+._:/=~-", rune(char)) {
			return false
		}
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	if strings.Contains(name, ":/") {
		return false
	}
	return true
}

func isPackageASCIIAlphaNumeric(char byte) bool {
	return isPackageASCIIAlpha(char) || (char >= '0' && char <= '9')
}

func isPackageASCIIAlpha(char byte) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
}
