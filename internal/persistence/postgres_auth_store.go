package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/idgen"
)

const authStoreQueryTimeout = 5 * time.Second
const authUserSelectColumns = `id, username, role, auth_provider, oidc_subject, password_hash, totp_secret, totp_verified_at, totp_recovery_codes, created_at, updated_at`

func authStoreContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), authStoreQueryTimeout)
}

func (s *PostgresStore) GetUserByID(id string) (auth.User, bool, error) {
	ctx, cancel := authStoreContext()
	defer cancel()

	user, err := scanUser(s.pool.QueryRow(ctx,
		`SELECT `+authUserSelectColumns+`
		 FROM users WHERE id = $1`,
		strings.TrimSpace(id),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.User{}, false, nil
		}
		return auth.User{}, false, err
	}
	return user, true, nil
}

func (s *PostgresStore) GetUserByUsername(username string) (auth.User, bool, error) {
	ctx, cancel := authStoreContext()
	defer cancel()

	user, err := scanUser(s.pool.QueryRow(ctx,
		`SELECT `+authUserSelectColumns+`
		 FROM users WHERE username = $1`,
		strings.TrimSpace(username),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.User{}, false, nil
		}
		return auth.User{}, false, err
	}
	return user, true, nil
}

func (s *PostgresStore) GetUserByOIDCSubject(provider, subject string) (auth.User, bool, error) {
	ctx, cancel := authStoreContext()
	defer cancel()

	user, err := scanUser(s.pool.QueryRow(ctx,
		`SELECT `+authUserSelectColumns+`
		 FROM users
		 WHERE auth_provider = $1 AND oidc_subject = $2`,
		strings.TrimSpace(provider),
		strings.TrimSpace(subject),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.User{}, false, nil
		}
		return auth.User{}, false, err
	}
	return user, true, nil
}

func (s *PostgresStore) ListUsers(limit int) ([]auth.User, error) {
	ctx, cancel := authStoreContext()
	defer cancel()
	if limit <= 0 || limit > 500 {
		limit = 200
	}

	rows, err := s.pool.Query(ctx,
		`SELECT `+authUserSelectColumns+`
		 FROM users
		 ORDER BY created_at ASC
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]auth.User, 0)
	for rows.Next() {
		user, scanErr := scanUser(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		users = append(users, user)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return users, nil
}

func (s *PostgresStore) CreateUser(username, passwordHash string) (auth.User, error) {
	return s.CreateUserWithRole(username, passwordHash, auth.RoleOwner, "local", "")
}

func (s *PostgresStore) BootstrapFirstUser(username, passwordHash string) (auth.User, bool, error) {
	ctx, cancel := authStoreContext()
	defer cancel()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return auth.User{}, false, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1, $2)`, 0x4c54, 0x4255); err != nil {
		return auth.User{}, false, err
	}

	var userCount int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&userCount); err != nil {
		return auth.User{}, false, err
	}
	if userCount > 0 {
		return auth.User{}, false, nil
	}

	now := time.Now().UTC()
	user, err := scanUser(tx.QueryRow(ctx,
		`INSERT INTO users (id, username, role, auth_provider, oidc_subject, password_hash, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $7)
		 RETURNING `+authUserSelectColumns,
		idgen.New("usr"),
		strings.TrimSpace(username),
		auth.RoleOwner,
		"local",
		"",
		passwordHash,
		now,
	))
	if err != nil {
		return auth.User{}, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return auth.User{}, false, err
	}
	return user, true, nil
}

func (s *PostgresStore) CreateUserWithRole(username, passwordHash, role, authProvider, oidcSubject string) (auth.User, error) {
	now := time.Now().UTC()
	ctx, cancel := authStoreContext()
	defer cancel()
	authProvider = strings.ToLower(strings.TrimSpace(authProvider))
	if authProvider == "" {
		authProvider = "local"
	}

	return scanUser(s.pool.QueryRow(ctx,
		`INSERT INTO users (id, username, role, auth_provider, oidc_subject, password_hash, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $7)
		 RETURNING `+authUserSelectColumns,
		idgen.New("usr"),
		strings.TrimSpace(username),
		auth.NormalizeRole(role),
		authProvider,
		strings.TrimSpace(oidcSubject),
		passwordHash,
		now,
	))
}

func (s *PostgresStore) UpdateUserPasswordHash(id, passwordHash string) error {
	now := time.Now().UTC()
	ctx, cancel := authStoreContext()
	defer cancel()

	_, err := s.pool.Exec(ctx,
		`UPDATE users
		 SET password_hash = $1, updated_at = $2
		 WHERE id = $3`,
		passwordHash,
		now,
		strings.TrimSpace(id),
	)
	return err
}

func (s *PostgresStore) UpdateUserRole(id, role string) error {
	now := time.Now().UTC()
	ctx, cancel := authStoreContext()
	defer cancel()

	_, err := s.pool.Exec(ctx,
		`UPDATE users
		 SET role = $1, updated_at = $2
		 WHERE id = $3`,
		auth.NormalizeRole(role),
		now,
		strings.TrimSpace(id),
	)
	return err
}

func (s *PostgresStore) DeleteUser(id string) error {
	ctx, cancel := authStoreContext()
	defer cancel()
	result, err := s.pool.Exec(ctx,
		`DELETE FROM users WHERE id = $1`, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

func (s *PostgresStore) ListSessionsByUserID(userID string) ([]auth.Session, error) {
	ctx, cancel := authStoreContext()
	defer cancel()
	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, token_hash, expires_at, created_at FROM sessions WHERE user_id = $1 AND expires_at > NOW() ORDER BY created_at DESC`,
		strings.TrimSpace(userID))
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []auth.Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}

func (s *PostgresStore) CreateAuthSession(userID, tokenHash string, expiresAt time.Time) (auth.Session, error) {
	now := time.Now().UTC()
	ctx, cancel := authStoreContext()
	defer cancel()

	return scanSession(s.pool.QueryRow(ctx,
		`INSERT INTO sessions (id, user_id, token_hash, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, user_id, token_hash, expires_at, created_at`,
		idgen.New("sess"),
		userID,
		tokenHash,
		expiresAt,
		now,
	))
}

func (s *PostgresStore) ValidateSession(tokenHash string) (auth.Session, bool, error) {
	ctx, cancel := authStoreContext()
	defer cancel()

	session, err := scanSession(s.pool.QueryRow(ctx,
		`SELECT id, user_id, token_hash, expires_at, created_at
		 FROM sessions
		 WHERE token_hash = $1 AND expires_at > $2`,
		tokenHash,
		time.Now().UTC(),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.Session{}, false, nil
		}
		return auth.Session{}, false, err
	}
	return session, true, nil
}

func (s *PostgresStore) DeleteSession(id string) error {
	ctx, cancel := authStoreContext()
	defer cancel()

	_, err := s.pool.Exec(ctx,
		`DELETE FROM sessions WHERE id = $1`,
		strings.TrimSpace(id),
	)
	return err
}

func (s *PostgresStore) DeleteSessionsByUserID(userID string) error {
	ctx, cancel := authStoreContext()
	defer cancel()

	_, err := s.pool.Exec(ctx,
		`DELETE FROM sessions WHERE user_id = $1`,
		strings.TrimSpace(userID),
	)
	return err
}

func (s *PostgresStore) DeleteExpiredSessions() (int64, error) {
	ctx, cancel := authStoreContext()
	defer cancel()

	tag, err := s.pool.Exec(ctx,
		`DELETE FROM sessions WHERE expires_at <= $1`,
		time.Now().UTC(),
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PostgresStore) SetUserTOTPSecret(id, encryptedSecret string) error {
	ctx, cancel := authStoreContext()
	defer cancel()
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET totp_secret = $1, updated_at = $2 WHERE id = $3`,
		encryptedSecret, time.Now().UTC(), strings.TrimSpace(id))
	return err
}

func (s *PostgresStore) ConfirmUserTOTP(id, recoveryCodes string) error {
	ctx, cancel := authStoreContext()
	defer cancel()
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET totp_verified_at = $1, totp_recovery_codes = $2, updated_at = $1 WHERE id = $3`,
		now, recoveryCodes, strings.TrimSpace(id))
	return err
}

func (s *PostgresStore) ClearUserTOTP(id string) error {
	ctx, cancel := authStoreContext()
	defer cancel()
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET totp_secret = NULL, totp_verified_at = NULL, totp_recovery_codes = NULL, updated_at = $1 WHERE id = $2`,
		time.Now().UTC(), strings.TrimSpace(id))
	return err
}

func (s *PostgresStore) UpdateUserRecoveryCodes(id, recoveryCodes string) error {
	ctx, cancel := authStoreContext()
	defer cancel()
	_, err := s.pool.Exec(ctx,
		`UPDATE users SET totp_recovery_codes = $1, updated_at = $2 WHERE id = $3`,
		recoveryCodes, time.Now().UTC(), strings.TrimSpace(id))
	return err
}

// ConsumeRecoveryCode atomically checks and removes a matching recovery code for the given user.
// It returns true (and commits the removal) if a matching code was found, false otherwise.
func (s *PostgresStore) ConsumeRecoveryCode(userID, code string) (bool, error) {
	ctx, cancel := authStoreContext()
	defer cancel()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var recoveryCodes string
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(totp_recovery_codes, '') FROM users WHERE id = $1 FOR UPDATE`,
		strings.TrimSpace(userID),
	).Scan(&recoveryCodes)
	if err != nil {
		return false, err
	}
	if recoveryCodes == "" {
		return false, nil
	}

	var hashes []string
	if err := json.Unmarshal([]byte(recoveryCodes), &hashes); err != nil {
		return false, nil
	}

	matchIdx := -1
	for i, hash := range hashes {
		if auth.CheckRecoveryCode(code, hash) {
			matchIdx = i
			break
		}
	}
	if matchIdx < 0 {
		return false, nil
	}

	hashes = append(hashes[:matchIdx], hashes[matchIdx+1:]...)
	updated, _ := json.Marshal(hashes)

	_, err = tx.Exec(ctx,
		`UPDATE users SET totp_recovery_codes = $1, updated_at = $2 WHERE id = $3`,
		string(updated), time.Now().UTC(), strings.TrimSpace(userID))
	if err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit recovery code consumption: %w", err)
	}
	return true, nil
}
