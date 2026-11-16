package main

import (
	"context"
	"net/http"

	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/servicehttp"
)

// The context key type, constants and accessors live in internal/apiv2.

// contextWithPrincipal also tells the request's audit row who signed in.
func contextWithPrincipal(ctx context.Context, userID, role string) context.Context {
	noteAuditActor(ctx, userID)
	return apiv2.ContextWithPrincipal(ctx, userID, role)
}

// requireAdminAuth checks that the request has admin privileges.
// If not, it writes a 403 Forbidden response and returns false.
// Use as an early-return guard in destructive action handlers:
//
//	if !s.requireAdminAuth(w, r) {
//	    return
//	}
func (s *apiServer) requireAdminAuth(w http.ResponseWriter, r *http.Request) bool {
	if !auth.HasAdminPrivileges(apiv2.UserRoleFromContext(r.Context())) {
		servicehttp.WriteError(w, http.StatusForbidden, "forbidden")
		return false
	}
	return true
}
