package shared

import "strings"

// SSHLoginUser returns name as an SSH login, or "" when it cannot be one: a
// Windows machine account (ending in "$", such as WORKGROUP\HOST$) or
// NT AUTHORITY\SYSTEM, which an agent running as a service may report.
func SSHLoginUser(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasSuffix(name, "$") || strings.EqualFold(name, `NT AUTHORITY\SYSTEM`) {
		return ""
	}
	return name
}
