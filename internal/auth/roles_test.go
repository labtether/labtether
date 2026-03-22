package auth

import "testing"

func TestNormalizeRole(t *testing.T) {
	if got := NormalizeRole("ADMIN"); got != RoleAdmin {
		t.Fatalf("expected admin, got %q", got)
	}
	if got := NormalizeRole("unknown"); got != RoleViewer {
		t.Fatalf("expected fallback viewer, got %q", got)
	}
}

func TestPrivileges(t *testing.T) {
	if !HasAdminPrivileges(RoleOwner) {
		t.Fatalf("owner should have admin privileges")
	}
	if HasAdminPrivileges(RoleOperator) {
		t.Fatalf("operator should not have admin privileges")
	}
	if !HasWritePrivileges(RoleOperator) {
		t.Fatalf("operator should have write privileges")
	}
	if HasWritePrivileges(RoleViewer) {
		t.Fatalf("viewer should not have write privileges")
	}
}

func TestClaimValues(t *testing.T) {
	values := claimValues([]any{"admin", "", 1, "viewer"})
	if len(values) != 2 || values[0] != "admin" || values[1] != "viewer" {
		t.Fatalf("unexpected claim values: %#v", values)
	}
}
