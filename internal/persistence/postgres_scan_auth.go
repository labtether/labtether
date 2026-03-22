package persistence

import (
	"database/sql"

	"github.com/labtether/labtether/internal/auth"
)

type userScanner interface {
	Scan(dest ...any) error
}

func scanUser(row userScanner) (auth.User, error) {
	user := auth.User{}
	oidcSubject := sql.NullString{}
	totpSecret := sql.NullString{}
	totpVerifiedAt := sql.NullTime{}
	totpRecoveryCodes := sql.NullString{}
	if err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Role,
		&user.AuthProvider,
		&oidcSubject,
		&user.PasswordHash,
		&totpSecret,
		&totpVerifiedAt,
		&totpRecoveryCodes,
		&user.CreatedAt,
		&user.UpdatedAt,
	); err != nil {
		return auth.User{}, err
	}
	if oidcSubject.Valid {
		user.OIDCSubject = oidcSubject.String
	}
	if totpSecret.Valid {
		user.TOTPSecret = totpSecret.String
	}
	if totpVerifiedAt.Valid {
		t := totpVerifiedAt.Time.UTC()
		user.TOTPVerifiedAt = &t
	}
	if totpRecoveryCodes.Valid {
		user.TOTPRecoveryCodes = totpRecoveryCodes.String
	}
	user.CreatedAt = user.CreatedAt.UTC()
	user.UpdatedAt = user.UpdatedAt.UTC()
	return user, nil
}

type sessionScanner interface {
	Scan(dest ...any) error
}

func scanSession(row sessionScanner) (auth.Session, error) {
	session := auth.Session{}
	if err := row.Scan(
		&session.ID,
		&session.UserID,
		&session.TokenHash,
		&session.ExpiresAt,
		&session.CreatedAt,
	); err != nil {
		return auth.Session{}, err
	}
	session.ExpiresAt = session.ExpiresAt.UTC()
	session.CreatedAt = session.CreatedAt.UTC()
	return session, nil
}
