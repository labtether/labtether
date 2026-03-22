package model

import "time"

type IngestCheckpoint struct {
	ProviderInstanceID string    `json:"provider_instance_id"`
	Stream             string    `json:"stream"`
	Cursor             string    `json:"cursor,omitempty"`
	SyncedAt           time.Time `json:"synced_at"`
}

type ReconciliationResult struct {
	ProviderInstanceID string    `json:"provider_instance_id"`
	CreatedCount       int       `json:"created_count"`
	UpdatedCount       int       `json:"updated_count"`
	StaleCount         int       `json:"stale_count"`
	ErrorCount         int       `json:"error_count"`
	StartedAt          time.Time `json:"started_at"`
	FinishedAt         time.Time `json:"finished_at"`
}
