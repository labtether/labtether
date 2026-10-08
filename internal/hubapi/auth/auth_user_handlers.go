package auth

import (
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/servicehttp"
	"log"
	"net/http"
	"strings"
	"time"
)

type authCreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"` // #nosec G117 -- Request payload intentionally carries runtime credential material.
	Role     string `json:"role"`
}

type authUpdateUserRequest struct {
	Role     *string `json:"role,omitempty"`
	Password *string `json:"password,omitempty"` // #nosec G117 -- Request payload intentionally carries runtime credential material.
}

// HandleAuthUsers handles GET/POST /auth/users.
func (d *Deps) HandleAuthUsers(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != authUsersRoute {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.AuthStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}

	switch r.Method {
	case http.MethodGet:
		users, err := d.AuthStore.ListUsers(200)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list users")
			return
		}
		type userEntry struct {
			ID           string `json:"id"`
			Username     string `json:"username"`
			Role         string `json:"role"`
			AuthProvider string `json:"auth_provider"`
		}
		items := make([]userEntry, 0, len(users))
		for _, u := range users {
			items = append(items, userEntry{ID: u.ID, Username: u.Username, Role: auth.NormalizeRole(u.Role), AuthProvider: u.AuthProvider})
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"users": items})
	case http.MethodPost:
		if rejectAPIKeyIdentityMutation(w, r) {
			return
		}
		var req authCreateUserRequest
		if err := shared.DecodeJSONBody(w, r, &req); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, "invalid user payload")
			return
		}
		req.Username = NormalizeUsername(req.Username)
		role := strings.ToLower(strings.TrimSpace(req.Role))
		if role == "" {
			role = auth.RoleViewer
		}
		if !auth.IsValidRole(role) {
			servicehttp.WriteError(w, http.StatusBadRequest, "role must be owner, admin, operator, or viewer")
			return
		}
		req.Role = role
		if req.Username == "" {
			servicehttp.WriteError(w, http.StatusBadRequest, "username is required")
			return
		}
		if err := ValidateLoginRequest(LoginRequest{Username: req.Username, Password: req.Password}); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.Role == auth.RoleOwner {
			servicehttp.WriteError(w, http.StatusBadRequest, "owner role is reserved")
			return
		}
		if _, exists, err := d.AuthStore.GetUserByUsername(req.Username); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to validate username")
			return
		} else if exists {
			servicehttp.WriteError(w, http.StatusConflict, "username already exists")
			return
		}
		hash, err := auth.HashPassword(req.Password)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to hash password")
			return
		}
		created, err := d.AuthStore.CreateUserWithRole(req.Username, hash, req.Role, "local", "")
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to create user")
			return
		}
		servicehttp.WriteJSON(w, http.StatusCreated, map[string]any{
			"user": auth.UserInfo{ID: created.ID, Username: created.Username, Role: auth.NormalizeRole(created.Role)},
		})
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// HandleAuthUserActions handles PATCH /auth/users/{id}.
func (d *Deps) HandleAuthUserActions(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, authUsersRouteSlash) {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if d.AuthStore == nil {
		servicehttp.WriteError(w, http.StatusServiceUnavailable, "authentication unavailable")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, authUsersRouteSlash)
	id = strings.TrimSpace(strings.Trim(id, "/"))
	if id == "" {
		servicehttp.WriteError(w, http.StatusNotFound, "user id is required")
		return
	}

	if strings.HasSuffix(id, "/sessions") {
		userID := strings.TrimSuffix(id, "/sessions")
		if r.Method == http.MethodDelete && rejectAPIKeyIdentityMutation(w, r) {
			return
		}
		d.handleUserSessions(w, r, userID)
		return
	}

	switch r.Method {
	case http.MethodPatch:
		if rejectAPIKeyIdentityMutation(w, r) {
			return
		}
	case http.MethodDelete:
		if rejectAPIKeyIdentityMutation(w, r) {
			return
		}
		d.handleDeleteUser(w, r, id)
		return
	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req authUpdateUserRequest
	if err := shared.DecodeJSONBody(w, r, &req); err != nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "invalid user update payload")
		return
	}
	if req.Role == nil && req.Password == nil {
		servicehttp.WriteError(w, http.StatusBadRequest, "at least one field must be provided")
		return
	}

	user, ok, err := d.AuthStore.GetUserByID(id)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load user")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "user not found")
		return
	}

	if req.Role != nil {
		role := strings.ToLower(strings.TrimSpace(*req.Role))
		if !auth.IsValidRole(role) {
			servicehttp.WriteError(w, http.StatusBadRequest, "role must be owner, admin, operator, or viewer")
			return
		}
		if auth.NormalizeRole(user.Role) == auth.RoleOwner && role != auth.RoleOwner {
			servicehttp.WriteError(w, http.StatusBadRequest, "cannot change owner role")
			return
		}
		if role == auth.RoleOwner {
			servicehttp.WriteError(w, http.StatusBadRequest, "owner role is reserved")
			return
		}
		if err := d.AuthStore.UpdateUserRole(user.ID, role); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update user role")
			return
		}
		user.Role = role
	}
	if req.Password != nil {
		if auth.NormalizeRole(user.Role) == auth.RoleOwner && d.UserIDFromContext(r.Context()) != user.ID {
			servicehttp.WriteError(w, http.StatusForbidden, "cannot change owner password")
			return
		}
		password := *req.Password
		if err := ValidateLoginRequest(LoginRequest{Username: user.Username, Password: password}); err != nil {
			servicehttp.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to hash password")
			return
		}
		if err := d.AuthStore.DeleteSessionsByUserID(user.ID); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to revoke active sessions")
			return
		}
		if err := d.AuthStore.UpdateUserPasswordHash(user.ID, hash); err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to update password")
			return
		}
		if user.ID == d.UserIDFromContext(r.Context()) {
			auth.ClearSessionCookie(w)
		}
	}

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"user": auth.UserInfo{ID: user.ID, Username: user.Username, Role: auth.NormalizeRole(user.Role)},
	})
}

func (d *Deps) handleDeleteUser(w http.ResponseWriter, r *http.Request, id string) {
	callerID := d.UserIDFromContext(r.Context())
	if callerID == id {
		servicehttp.WriteError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	user, ok, err := d.AuthStore.GetUserByID(id)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to look up user")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	if user.Role == "owner" {
		servicehttp.WriteError(w, http.StatusForbidden, "cannot delete the owner account")
		return
	}

	if delErr := d.AuthStore.DeleteSessionsByUserID(id); delErr != nil {
		log.Printf("auth: delete-user: failed to revoke sessions for user %s: %v", id, delErr) // #nosec G706 -- User IDs are store-generated identifiers and the error is local runtime state.
	}

	if err := d.AuthStore.DeleteUser(id); err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (d *Deps) handleUserSessions(w http.ResponseWriter, r *http.Request, userID string) {
	if userID == "" {
		servicehttp.WriteError(w, http.StatusBadRequest, "missing user id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		sessions, err := d.AuthStore.ListSessionsByUserID(userID)
		if err != nil {
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to list sessions")
			return
		}
		if sessions == nil {
			sessions = []auth.Session{}
		}
		type sessionInfo struct {
			ID        string    `json:"id"`
			CreatedAt time.Time `json:"created_at"`
			ExpiresAt time.Time `json:"expires_at"`
		}
		out := make([]sessionInfo, len(sessions))
		for i, s := range sessions {
			out[i] = sessionInfo{ID: s.ID, CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt}
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"sessions": out, "count": len(out)})

	case http.MethodDelete:
		if err := d.AuthStore.DeleteSessionsByUserID(userID); err != nil {
			log.Printf("auth: revoke-sessions: failed to delete sessions for user %s: %v", userID, err) // #nosec G706 -- User IDs are store-generated identifiers and the error is local runtime state.
			servicehttp.WriteError(w, http.StatusInternalServerError, "failed to revoke sessions")
			return
		}
		servicehttp.WriteJSON(w, http.StatusOK, map[string]any{"revoked": true})

	default:
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
