package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/labtether/labtether/internal/audit"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/terminal"
)

func (s *PostgresStore) CreateSession(req terminal.CreateSessionRequest) (terminal.Session, error) {
	now := time.Now().UTC()
	mode := req.Mode
	if mode == "" {
		mode = "interactive"
	}

	session := terminal.Session{
		ID:                  idgen.New("sess"),
		ActorID:             req.ActorID,
		Target:              req.Target,
		Mode:                mode,
		Status:              "active",
		PersistentSessionID: req.PersistentSessionID,
		TmuxSessionName:     req.TmuxSessionName,
		CreatedAt:           now,
		LastActionAt:        now,
	}

	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO terminal_sessions (id, actor_id, target, mode, status, persistent_session_id, tmux_session_name, created_at, last_action_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		session.ID, session.ActorID, session.Target, session.Mode, session.Status, session.PersistentSessionID, session.TmuxSessionName, session.CreatedAt, session.LastActionAt,
	)
	if err != nil {
		return terminal.Session{}, err
	}

	return session, nil
}

func (s *PostgresStore) GetSession(id string) (terminal.Session, bool, error) {
	row := s.pool.QueryRow(context.Background(),
		`SELECT id, actor_id, target, mode, status, persistent_session_id, tmux_session_name, created_at, last_action_at
		 FROM terminal_sessions WHERE id = $1`,
		id,
	)

	session := terminal.Session{}
	if err := row.Scan(
		&session.ID,
		&session.ActorID,
		&session.Target,
		&session.Mode,
		&session.Status,
		&session.PersistentSessionID,
		&session.TmuxSessionName,
		&session.CreatedAt,
		&session.LastActionAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return terminal.Session{}, false, nil
		}
		return terminal.Session{}, false, err
	}

	return session, true, nil
}

func (s *PostgresStore) ListSessions() ([]terminal.Session, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT id, actor_id, target, mode, status, persistent_session_id, tmux_session_name, created_at, last_action_at
		 FROM terminal_sessions ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]terminal.Session, 0, 32)
	for rows.Next() {
		session := terminal.Session{}
		if err := rows.Scan(
			&session.ID,
			&session.ActorID,
			&session.Target,
			&session.Mode,
			&session.Status,
			&session.PersistentSessionID,
			&session.TmuxSessionName,
			&session.CreatedAt,
			&session.LastActionAt,
		); err != nil {
			return nil, err
		}
		out = append(out, session)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return out, nil
}

func (s *PostgresStore) UpdateSession(session terminal.Session) error {
	// Update mutable fields. Note: Source and InlineSSHConfig are in-memory only
	// and not persisted to Postgres (no source column in terminal_sessions).
	_, err := s.pool.Exec(context.Background(),
		`UPDATE terminal_sessions SET status = $2, last_action_at = $3 WHERE id = $1`,
		session.ID, session.Status, session.LastActionAt,
	)
	return err
}

func (s *PostgresStore) DeleteTerminalSession(id string) error {
	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	if _, err := tx.Exec(context.Background(), `DELETE FROM terminal_commands WHERE session_id = $1`, id); err != nil {
		return err
	}

	result, err := tx.Exec(context.Background(), `DELETE FROM terminal_sessions WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return terminal.ErrSessionNotFound
	}

	return tx.Commit(context.Background())
}

func (s *PostgresStore) AddCommand(sessionID string, req terminal.CreateCommandRequest, target, mode string) (terminal.Command, error) {
	now := time.Now().UTC()
	command := terminal.Command{
		ID:        idgen.New("cmd"),
		SessionID: sessionID,
		ActorID:   req.ActorID,
		Target:    target,
		Body:      req.Command,
		Mode:      mode,
		Status:    "queued",
		CreatedAt: now,
		UpdatedAt: now,
	}

	tx, err := s.pool.Begin(context.Background())
	if err != nil {
		return terminal.Command{}, err
	}
	defer tx.Rollback(context.Background())

	var sessionExists bool
	if err := tx.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM terminal_sessions WHERE id = $1)`,
		sessionID,
	).Scan(&sessionExists); err != nil {
		return terminal.Command{}, err
	}

	if !sessionExists {
		return terminal.Command{}, terminal.ErrSessionNotFound
	}

	if _, err := tx.Exec(context.Background(),
		`INSERT INTO terminal_commands (id, session_id, actor_id, target, body, mode, status, output, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, '', $8, $9)`,
		command.ID,
		command.SessionID,
		command.ActorID,
		command.Target,
		command.Body,
		command.Mode,
		command.Status,
		command.CreatedAt,
		command.UpdatedAt,
	); err != nil {
		return terminal.Command{}, err
	}

	if _, err := tx.Exec(context.Background(),
		`UPDATE terminal_sessions SET last_action_at = $2 WHERE id = $1`,
		sessionID,
		now,
	); err != nil {
		return terminal.Command{}, err
	}

	if err := tx.Commit(context.Background()); err != nil {
		return terminal.Command{}, err
	}

	return command, nil
}

func (s *PostgresStore) UpdateCommandResult(sessionID, commandID, status, output string) error {
	cmdTag, err := s.pool.Exec(context.Background(),
		`UPDATE terminal_commands
		 SET status = $3, output = $4, updated_at = $5
		 WHERE session_id = $1 AND id = $2`,
		sessionID,
		commandID,
		status,
		output,
		time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return errors.New("command not found")
	}

	_, _ = s.pool.Exec(context.Background(),
		`UPDATE terminal_sessions SET last_action_at = $2 WHERE id = $1`,
		sessionID,
		time.Now().UTC(),
	)
	return nil
}

func (s *PostgresStore) ListCommands(sessionID string) ([]terminal.Command, error) {
	var exists bool
	if err := s.pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM terminal_sessions WHERE id = $1)`,
		sessionID,
	).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, terminal.ErrSessionNotFound
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT id, session_id, actor_id, target, body, mode, status, output, created_at, updated_at
		 FROM terminal_commands WHERE session_id = $1 ORDER BY created_at DESC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	commands := make([]terminal.Command, 0, 32)
	for rows.Next() {
		cmd := terminal.Command{}
		if err := rows.Scan(
			&cmd.ID,
			&cmd.SessionID,
			&cmd.ActorID,
			&cmd.Target,
			&cmd.Body,
			&cmd.Mode,
			&cmd.Status,
			&cmd.Output,
			&cmd.CreatedAt,
			&cmd.UpdatedAt,
		); err != nil {
			return nil, err
		}
		commands = append(commands, cmd)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return commands, nil
}

func (s *PostgresStore) ListRecentCommands(limit int) ([]terminal.Command, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.pool.Query(context.Background(),
		`SELECT id, session_id, actor_id, target, body, mode, status, output, created_at, updated_at
		 FROM terminal_commands ORDER BY updated_at DESC LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	commands := make([]terminal.Command, 0)
	for rows.Next() {
		cmd := terminal.Command{}
		if err := rows.Scan(
			&cmd.ID,
			&cmd.SessionID,
			&cmd.ActorID,
			&cmd.Target,
			&cmd.Body,
			&cmd.Mode,
			&cmd.Status,
			&cmd.Output,
			&cmd.CreatedAt,
			&cmd.UpdatedAt,
		); err != nil {
			return nil, err
		}
		commands = append(commands, cmd)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return commands, nil
}

func (s *PostgresStore) Append(event audit.Event) error {
	if event.ID == "" {
		event.ID = idgen.New("audit")
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	details := []byte("{}")
	if event.Details != nil {
		payload, err := json.Marshal(event.Details)
		if err != nil {
			return err
		}
		details = payload
	}

	_, err := s.pool.Exec(context.Background(),
		`INSERT INTO audit_events (id, type, actor_id, target, session_id, command_id, decision, reason, details, timestamp)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10)
		 ON CONFLICT (id) DO NOTHING`,
		event.ID,
		event.Type,
		event.ActorID,
		event.Target,
		event.SessionID,
		event.CommandID,
		event.Decision,
		event.Reason,
		string(details),
		event.Timestamp,
	)
	return err
}

func (s *PostgresStore) List(limit, offset int) ([]audit.Event, error) {
	if limit <= 0 {
		limit = 100
	}

	var rows pgx.Rows
	var err error
	if offset > 0 {
		rows, err = s.pool.Query(context.Background(),
			`SELECT id, type, actor_id, target, session_id, command_id, decision, reason, details, timestamp
			 FROM audit_events ORDER BY timestamp DESC LIMIT $1 OFFSET $2`,
			limit, offset,
		)
	} else {
		rows, err = s.pool.Query(context.Background(),
			`SELECT id, type, actor_id, target, session_id, command_id, decision, reason, details, timestamp
			 FROM audit_events ORDER BY timestamp DESC LIMIT $1`,
			limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]audit.Event, 0)
	for rows.Next() {
		event := audit.Event{}
		var details []byte
		if err := rows.Scan(
			&event.ID,
			&event.Type,
			&event.ActorID,
			&event.Target,
			&event.SessionID,
			&event.CommandID,
			&event.Decision,
			&event.Reason,
			&details,
			&event.Timestamp,
		); err != nil {
			return nil, err
		}

		if len(details) > 0 {
			mapped := map[string]any{}
			if err := json.Unmarshal(details, &mapped); err == nil {
				event.Details = mapped
			}
		}
		out = append(out, event)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.After(out[j].Timestamp)
	})
	return out, nil
}
