package persistence

// schemaMigrationCommandExitCodes stores a command's real exit status for
// terminal commands and action run steps. NULL means unknown: an older agent,
// a timeout, or a command that never ran. It is never stored as 0 then.
// BIGINT holds every Windows exit code, up to 4294967295.
func schemaMigrationCommandExitCodes() schemaMigration {
	return schemaMigration{Version: 108, Name: "command_exit_codes", Statements: []string{
		`ALTER TABLE terminal_commands ADD COLUMN IF NOT EXISTS exit_code BIGINT`,
		`ALTER TABLE action_run_steps ADD COLUMN IF NOT EXISTS exit_code BIGINT`,
	}}
}
