package assets

import (
	"errors"
	"strings"
	"unicode"
)

// ErrNameHasControlCharacters refuses a device name with a line break, tab or
// other control character. Such a name broke email subjects and one-line
// lists, and showed as "nas01 Bcc: x@example.com".
var ErrNameHasControlCharacters = errors.New("The name can't contain line breaks, tabs or other control characters.")

// ValidateName checks a device name a person typed.
func ValidateName(name string) error {
	if strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return ErrNameHasControlCharacters
	}
	return nil
}

// OneLineName is a name as one line: control characters become spaces,
// runs of spaces become one, and the ends are trimmed. Names that agents and
// connectors report are stored this way.
func OneLineName(name string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, name)), " ")
}
