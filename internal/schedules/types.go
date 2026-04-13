package schedules

import "time"

// ScheduledTask represents a saved recurring-command definition stored in the hub.
// The cron expression is persisted for API/export workflows, but the hub does not
// automatically evaluate or execute scheduled tasks yet.
type ScheduledTask struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	CronExpr  string     `json:"cron_expr"`
	Command   string     `json:"command"`
	Targets   []string   `json:"targets"`
	GroupID   string     `json:"group_id,omitempty"`
	Enabled   bool       `json:"enabled"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	LastRunAt *time.Time `json:"last_run_at,omitempty"`
	NextRunAt *time.Time `json:"next_run_at,omitempty"`
}

// CreateRequest is the API request body for creating a scheduled task.
type CreateRequest struct {
	Name     string   `json:"name"`
	CronExpr string   `json:"cron_expr"`
	Command  string   `json:"command"`
	Targets  []string `json:"targets,omitempty"`
	GroupID  string   `json:"group_id,omitempty"`
}
