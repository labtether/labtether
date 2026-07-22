package alerting

import (
	"errors"
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/alerts"
)

func ValidateSilenceRequest(req alerts.CreateSilenceRequest) error {
	if len(req.Matchers) == 0 {
		return errors.New("matchers are required")
	}
	if len(req.Matchers) > MaxSilenceMatcherCount {
		return fmt.Errorf("matchers must contain no more than %d entries", MaxSilenceMatcherCount)
	}
	for key, value := range req.Matchers {
		if strings.TrimSpace(key) == "" {
			return errors.New("matcher labels cannot be empty")
		}
		if len([]rune(key)) > MaxSilenceMatcherKeyLen {
			return fmt.Errorf("matcher labels must be %d characters or fewer", MaxSilenceMatcherKeyLen)
		}
		if len([]rune(value)) > MaxSilenceMatcherValLen {
			return fmt.Errorf("matcher values must be %d characters or fewer", MaxSilenceMatcherValLen)
		}
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
