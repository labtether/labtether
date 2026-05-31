package mcpserver

import (
	"fmt"
	"regexp"
	"strings"
)

var safeShellAtomPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@:-]*$`)

func validateSafeShellAtom(name, value string) error {
	if value == "" || strings.TrimSpace(value) != value || !safeShellAtomPattern.MatchString(value) {
		return fmt.Errorf("invalid %s: only letters, numbers, dot, underscore, dash, colon, and at sign are allowed, and it must start with a letter or number", name)
	}
	return nil
}
