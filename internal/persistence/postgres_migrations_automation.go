package persistence

// Schema changes owned by the automation domain.

func schemaMigrationJobQueue() schemaMigration {
	return schemaMigration{
		Version: 9,
		Name:    "job_queue",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS job_queue (
					id TEXT PRIMARY KEY,
					kind TEXT NOT NULL,
					status TEXT NOT NULL DEFAULT 'queued',
					payload JSONB NOT NULL,
					attempts INT NOT NULL DEFAULT 0,
					max_attempts INT NOT NULL DEFAULT 5,
					error TEXT NOT NULL DEFAULT '',
					created_at TIMESTAMPTZ NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL,
					available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
					locked_at TIMESTAMPTZ,
					completed_at TIMESTAMPTZ,
					lock_token TEXT
				)`,
			`CREATE INDEX IF NOT EXISTS idx_job_queue_status_created ON job_queue(status, created_at ASC)`,
			`CREATE INDEX IF NOT EXISTS idx_job_queue_status_available_created ON job_queue(status, available_at ASC, created_at ASC)`,
			`CREATE INDEX IF NOT EXISTS idx_job_queue_kind_status ON job_queue(kind, status)`,
			`CREATE INDEX IF NOT EXISTS idx_job_queue_dead_lettered ON job_queue(created_at DESC) WHERE status = 'dead_lettered'`,
		},
	}
}

func schemaMigrationWebhooks() schemaMigration {
	return schemaMigration{
		Version: 66,
		Name:    "webhooks",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS webhooks (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				url TEXT NOT NULL,
				secret TEXT,
				events JSONB NOT NULL DEFAULT '[]',
				enabled BOOLEAN NOT NULL DEFAULT true,
				created_by TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				last_triggered_at TIMESTAMPTZ
			)`,
		},
	}
}

func schemaMigrationScheduledTasks() schemaMigration {
	return schemaMigration{
		Version: 67,
		Name:    "scheduled_tasks",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS scheduled_tasks (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				cron_expr TEXT NOT NULL,
				command TEXT NOT NULL,
				targets JSONB NOT NULL DEFAULT '[]',
				group_id TEXT,
				enabled BOOLEAN NOT NULL DEFAULT true,
				created_by TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				last_run_at TIMESTAMPTZ,
				next_run_at TIMESTAMPTZ
			)`,
		},
	}
}

func schemaMigrationSavedActions() schemaMigration {
	return schemaMigration{
		Version: 68,
		Name:    "saved_actions",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS saved_actions (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL,
				description TEXT,
				steps JSONB NOT NULL DEFAULT '[]',
				created_by TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL
			)`,
		},
	}
}

func schemaMigrationWebhookSecretCiphertext() schemaMigration {
	return schemaMigration{
		Version: 75,
		Name:    "webhook_secret_ciphertext",
		Statements: []string{
			`ALTER TABLE webhooks ADD COLUMN IF NOT EXISTS secret_ciphertext TEXT NOT NULL DEFAULT ''`,
			`UPDATE webhooks SET secret_ciphertext = '' WHERE secret_ciphertext IS NULL`,
		},
	}
}

func schemaMigrationJobQueueLeasesAndBackoff() schemaMigration {
	return schemaMigration{
		Version: 78,
		Name:    "job_queue_leases_and_backoff",
		Statements: []string{
			`ALTER TABLE job_queue ADD COLUMN IF NOT EXISTS available_at TIMESTAMPTZ NOT NULL DEFAULT now()`,
			`UPDATE job_queue SET available_at = created_at WHERE available_at IS NULL`,
			`ALTER TABLE job_queue ADD COLUMN IF NOT EXISTS lock_token TEXT`,
			`CREATE INDEX IF NOT EXISTS idx_job_queue_status_available_created ON job_queue(status, available_at ASC, created_at ASC)`,
		},
	}
}

func schemaMigrationSavedActionsOwnerIndex() schemaMigration {
	return schemaMigration{
		Version: 79,
		Name:    "saved_actions_owner_index",
		Statements: []string{
			`CREATE INDEX IF NOT EXISTS idx_saved_actions_created_by_created_at ON saved_actions(created_by, created_at DESC)`,
		},
	}
}

func schemaMigrationScheduledTaskExecutionState() schemaMigration {
	return schemaMigration{
		Version: 90,
		Name:    "scheduled_task_execution_state",
		Statements: []string{
			`ALTER TABLE scheduled_tasks ADD COLUMN IF NOT EXISTS last_run_status TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE scheduled_tasks ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE scheduled_tasks ADD COLUMN IF NOT EXISTS last_run_job_id TEXT`,
			`CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_enabled_next_run
				ON scheduled_tasks(next_run_at ASC NULLS FIRST, id ASC)
				WHERE enabled = TRUE`,
		},
	}
}
