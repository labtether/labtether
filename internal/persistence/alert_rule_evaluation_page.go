package persistence

import (
	"context"
	"errors"

	"github.com/labtether/labtether/internal/alerts"
)

// A cursor bounds one finite ID range, not a cross-page database snapshot.
// Edits do not move IDs. New or newly active IDs behind AfterID are revisited
// on the next sweep; a restart may safely repeat due-checked considerations.
type AlertRuleEvaluationCursor struct {
	Started   bool
	AfterID   string
	ThroughID string
}

type AlertRuleEvaluationPage struct {
	Rules     []alerts.Rule
	ThroughID string
	Done      bool
}

const AlertRuleEvaluationPageSize = 128

type AlertRuleEvaluationPageStore interface {
	PageActiveAlertRules(context.Context, AlertRuleEvaluationCursor, int) (AlertRuleEvaluationPage, error)
}

func validateAlertRuleEvaluationPage(cursor AlertRuleEvaluationCursor, limit int) error {
	if limit < 1 || limit > AlertRuleEvaluationPageSize {
		return errors.New("invalid alert evaluation page size")
	}
	if (!cursor.Started && (cursor.AfterID != "" || cursor.ThroughID != "")) || cursor.AfterID > cursor.ThroughID {
		return errors.New("invalid alert evaluation cursor")
	}
	return nil
}
