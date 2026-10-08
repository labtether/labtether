package auth

import (
	"errors"
	"fmt"
	"github.com/labtether/labtether/internal/auth"
	"strings"
)

// ResolveOIDCUser finds or creates a user from an OIDC identity.
func (d *Deps) ResolveOIDCUser(identity auth.OIDCIdentity) (auth.User, bool, error) {
	const provider = "oidc"
	issuer := strings.TrimSpace(identity.Issuer)
	subject := strings.TrimSpace(identity.Subject)
	if issuer == "" || subject == "" {
		return auth.User{}, false, errors.New("verified oidc issuer and subject are required")
	}
	desiredRole := OIDCAssignableRole(identity.Role)

	user, ok, err := d.AuthStore.GetUserByOIDCIdentity(provider, issuer, subject)
	if err != nil {
		return auth.User{}, false, err
	}
	if ok {
		if desiredRole != auth.NormalizeRole(user.Role) && auth.NormalizeRole(user.Role) != auth.RoleOwner {
			if updateErr := d.AuthStore.UpdateUserRole(user.ID, desiredRole); updateErr == nil {
				user.Role = desiredRole
			}
		}
		return user, false, nil
	}

	// Users provisioned before issuer-scoped identities have a subject but no
	// issuer. Adopt that exact row once, atomically. The first adoption preserves
	// its current role instead of applying a claim-driven role change as an
	// incidental side effect of the schema migration. Later logins follow the
	// existing role synchronization policy above.
	legacyUser, legacyOK, err := d.AuthStore.GetUserByOIDCSubject(provider, subject)
	if err != nil {
		return auth.User{}, false, err
	}
	if legacyOK {
		boundUser, bound, bindErr := d.AuthStore.BindLegacyOIDCIdentity(
			legacyUser.ID,
			provider,
			subject,
			issuer,
		)
		if bindErr == nil && bound {
			return boundUser, false, nil
		}

		// A concurrent login for the same verified issuer may have completed
		// the one-time binding. Accept it only when it is the same user; a
		// competing issuer or a different user fails closed.
		currentUser, currentOK, currentErr := d.AuthStore.GetUserByOIDCIdentity(provider, issuer, subject)
		if currentErr != nil {
			return auth.User{}, false, currentErr
		}
		if currentOK && currentUser.ID == legacyUser.ID {
			return currentUser, false, nil
		}
		if bindErr != nil {
			return auth.User{}, false, bindErr
		}
		return auth.User{}, false, errors.New("legacy oidc identity was bound by a competing issuer")
	}

	_, oidcAutoProvision := d.OIDCRef.Get()
	if !oidcAutoProvision {
		return auth.User{}, false, fmt.Errorf("oidc user %q is not provisioned", subject)
	}
	if setupRequired, setupErr := AuthBootstrapSetupRequired(d.AuthStore); setupErr != nil {
		return auth.User{}, false, setupErr
	} else if setupRequired {
		return auth.User{}, false, ErrOIDCSetupRequired
	}

	username := SelectOIDCUsername(identity)
	username, err = d.UniqueUsername(username)
	if err != nil {
		return auth.User{}, false, err
	}
	passwordHash, err := auth.HashPassword(GenerateSyntheticOIDCPassword())
	if err != nil {
		return auth.User{}, false, err
	}
	createdUser, err := d.AuthStore.CreateUserWithOIDCIdentity(
		username,
		passwordHash,
		desiredRole,
		provider,
		issuer,
		subject,
	)
	if err != nil {
		return auth.User{}, false, err
	}
	return createdUser, true, nil
}

// OIDCAssignableRole downgrades owner to admin for OIDC-provisioned users.
func OIDCAssignableRole(role string) string {
	normalized := auth.NormalizeRole(role)
	if normalized == auth.RoleOwner {
		return auth.RoleAdmin
	}
	return normalized
}

// GenerateSyntheticOIDCPassword generates a random password for OIDC-provisioned users.
func GenerateSyntheticOIDCPassword() string {
	token, err := RandomURLToken(48)
	if err != nil {
		return "oidc-fallback-password-not-used"
	}
	return token
}

// SelectOIDCUsername picks the best username from an OIDC identity.
func SelectOIDCUsername(identity auth.OIDCIdentity) string {
	for _, candidate := range []string{identity.PreferredUsername, identity.Email, identity.Name, identity.Subject} {
		normalized := NormalizeUsername(candidate)
		if normalized != "" {
			return normalized
		}
	}
	return "oidc-user"
}

// NormalizeUsername sanitizes and normalizes a username string.
func NormalizeUsername(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}
	if at := strings.Index(raw, "@"); at > 0 {
		raw = raw[:at]
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "-._")
	if out == "" {
		return ""
	}
	if len(out) > 32 {
		out = out[:32]
	}
	return out
}

// UniqueUsername generates a unique username by appending numeric suffixes.
func (d *Deps) UniqueUsername(base string) (string, error) {
	candidate := NormalizeUsername(base)
	if candidate == "" {
		candidate = "oidc-user"
	}
	for i := 0; i < 100; i++ {
		name := candidate
		if i > 0 {
			name = fmt.Sprintf("%s-%d", candidate, i+1)
		}
		if len(name) > 64 {
			name = name[:64]
		}
		_, exists, err := d.AuthStore.GetUserByUsername(name)
		if err != nil {
			return "", err
		}
		if !exists {
			return name, nil
		}
	}
	return "", fmt.Errorf("unable to allocate username for oidc identity")
}
