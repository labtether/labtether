package alerting

import (
	"errors"

	"github.com/labtether/labtether/internal/alerts"
)

func ValidateSilenceRequest(req alerts.CreateSilenceRequest) error {
	if len(req.Matchers) == 0 {
		return errors.New("matchers are required")
	}
	if req.StartsAt.IsZero() {
		return errors.New("starts_at is required")
	}
	if req.EndsAt.IsZero() {
		return errors.New("ends_at is required")
	}
	if !req.EndsAt.After(req.StartsAt) {
		return errors.New("ends_at must be after starts_at")
	}
	if err := validateMaxLen("reason", req.Reason, MaxAlertDescriptionLen); err != nil {
		return err
	}
	if err := validateMaxLen("created_by", req.CreatedBy, MaxActorIDLength); err != nil {
		return err
	}
	return nil
}
