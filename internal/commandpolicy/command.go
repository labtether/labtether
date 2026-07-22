// Package commandpolicy parses the deliberately small command language used
// by structured LabTether operations. It is not a shell grammar: operators,
// substitutions, redirects, and command chaining are rejected.
package commandpolicy

import (
	"fmt"
	"strings"
	"unicode"
)

// ParseArgv converts a structured command into argv without invoking a shell.
// Single/double quoting and backslash escaping are supported solely for
// grouping literal argument text.
func ParseArgv(raw string) ([]string, error) {
	var (
		argv         []string
		current      strings.Builder
		quote        rune
		escaped      bool
		tokenStarted bool
	)

	flush := func() {
		if tokenStarted {
			argv = append(argv, current.String())
			current.Reset()
			tokenStarted = false
		}
	}

	for _, r := range raw {
		if escaped {
			if r == '\n' || r == '\r' || r == 0 {
				return nil, fmt.Errorf("control characters are not allowed in commands")
			}
			current.WriteRune(r)
			tokenStarted = true
			escaped = false
			continue
		}

		if quote != 0 {
			if r == quote {
				quote = 0
				tokenStarted = true
				continue
			}
			if r == '\\' && quote == '"' {
				escaped = true
				continue
			}
			if r == '\n' || r == '\r' || r == 0 {
				return nil, fmt.Errorf("control characters are not allowed in commands")
			}
			current.WriteRune(r)
			tokenStarted = true
			continue
		}

		switch {
		case r == '\\':
			escaped = true
			tokenStarted = true
		case r == '\'' || r == '"':
			quote = r
			tokenStarted = true
		case unicode.IsControl(r) && r != '\t':
			return nil, fmt.Errorf("control characters are not allowed in commands")
		case unicode.IsSpace(r):
			flush()
		case strings.ContainsRune(";&|<>$`(){}", r):
			return nil, fmt.Errorf("shell operators and substitutions are not allowed")
		default:
			current.WriteRune(r)
			tokenStarted = true
		}
	}

	if escaped {
		return nil, fmt.Errorf("command ends with an incomplete escape")
	}
	if quote != 0 {
		return nil, fmt.Errorf("command contains an unterminated quote")
	}
	flush()
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return nil, fmt.Errorf("command is required")
	}
	return argv, nil
}

// MatchesRule requires every token in the allowlist rule to match an argv
// token exactly. Additional arguments are allowed after the matched rule.
func MatchesRule(argv []string, rule string) bool {
	ruleArgv, err := ParseArgv(rule)
	if err != nil || len(ruleArgv) == 0 || len(argv) < len(ruleArgv) {
		return false
	}
	for i := range ruleArgv {
		if !strings.EqualFold(argv[i], ruleArgv[i]) {
			return false
		}
	}
	return true
}

// QuoteArgv produces a shell-safe remote command string from already parsed
// argv. SSH servers necessarily pass command strings to a remote shell, so
// every argument is single-quoted and embedded quotes are escaped.
func QuoteArgv(argv []string) string {
	quoted := make([]string, 0, len(argv))
	for _, arg := range argv {
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "'\"'\"'")+"'")
	}
	return strings.Join(quoted, " ")
}
