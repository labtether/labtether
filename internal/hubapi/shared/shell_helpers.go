package shared

import "strings"

// ShellSingleQuote wraps value in POSIX-safe single quotes for use in
// shell command strings. Interior single quotes are escaped using the
// standard '"'"' idiom (end single-quote, double-quote a literal
// single-quote, restart single-quote).
func ShellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
