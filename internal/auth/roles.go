package auth

import "strings"

// NormalizeRole canonicalizes known role values and falls back to viewer.
func NormalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case RoleOwner:
		return RoleOwner
	case RoleAdmin:
		return RoleAdmin
	case RoleOperator:
		return RoleOperator
	default:
		return RoleViewer
	}
}

func HasAdminPrivileges(role string) bool {
	switch NormalizeRole(role) {
	case RoleOwner, RoleAdmin:
		return true
	default:
		return false
	}
}

func HasWritePrivileges(role string) bool {
	switch NormalizeRole(role) {
	case RoleOwner, RoleAdmin, RoleOperator:
		return true
	default:
		return false
	}
}

// LowerRole returns the less privileged of two roles.
func LowerRole(a, b string) string {
	rank := map[string]int{RoleViewer: 0, RoleOperator: 1, RoleAdmin: 2, RoleOwner: 3}
	a, b = NormalizeRole(a), NormalizeRole(b)
	if rank[b] < rank[a] {
		return b
	}
	return a
}
