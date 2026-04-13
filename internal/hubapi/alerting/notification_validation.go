package alerting

import (
	"errors"
	"strings"

	"github.com/labtether/labtether/internal/notifications"
)

func ValidateCreateChannelRequest(req notifications.CreateChannelRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return errors.New("name is required")
	}
	if err := validateMaxLen("name", req.Name, MaxAlertRuleNameLength); err != nil {
		return err
	}
	if notifications.NormalizeChannelType(req.Type) == "" {
		return errors.New("type must be webhook, email, slack, apns, ntfy, or gotify")
	}
	return nil
}

func ValidateCreateRouteRequest(req notifications.CreateRouteRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return errors.New("name is required")
	}
	if err := validateMaxLen("name", req.Name, MaxAlertRuleNameLength); err != nil {
		return err
	}
	if req.GroupWaitSeconds < 0 {
		return errors.New("group_wait_seconds must be >= 0")
	}
	if req.GroupIntervalSeconds < 0 {
		return errors.New("group_interval_seconds must be >= 0")
	}
	if req.RepeatIntervalSeconds < 0 {
		return errors.New("repeat_interval_seconds must be >= 0")
	}
	if err := ValidateNoDeprecatedCanonicalPredicateKeys(req.Matchers, "matchers"); err != nil {
		return err
	}
	if err := validateUnsupportedRouteDispatchSettings(req.GroupBy, req.GroupWaitSeconds, req.GroupIntervalSeconds, req.RepeatIntervalSeconds); err != nil {
		return err
	}
	return nil
}

func ValidateUpdateRouteRequest(req notifications.UpdateRouteRequest) error {
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return errors.New("name cannot be empty")
	}
	if req.GroupWaitSeconds != nil && *req.GroupWaitSeconds < 0 {
		return errors.New("group_wait_seconds must be >= 0")
	}
	if req.GroupIntervalSeconds != nil && *req.GroupIntervalSeconds < 0 {
		return errors.New("group_interval_seconds must be >= 0")
	}
	if req.RepeatIntervalSeconds != nil && *req.RepeatIntervalSeconds < 0 {
		return errors.New("repeat_interval_seconds must be >= 0")
	}
	if req.Matchers != nil {
		if err := ValidateNoDeprecatedCanonicalPredicateKeys(*req.Matchers, "matchers"); err != nil {
			return err
		}
	}
	groupBy := []string(nil)
	if req.GroupBy != nil {
		groupBy = *req.GroupBy
	}
	groupWait := 0
	if req.GroupWaitSeconds != nil {
		groupWait = *req.GroupWaitSeconds
	}
	groupInterval := 0
	if req.GroupIntervalSeconds != nil {
		groupInterval = *req.GroupIntervalSeconds
	}
	repeatInterval := 0
	if req.RepeatIntervalSeconds != nil {
		repeatInterval = *req.RepeatIntervalSeconds
	}
	if err := validateUnsupportedRouteDispatchSettings(groupBy, groupWait, groupInterval, repeatInterval); err != nil {
		return err
	}
	return nil
}

func validateUnsupportedRouteDispatchSettings(groupBy []string, groupWaitSeconds, groupIntervalSeconds, repeatIntervalSeconds int) error {
	if len(groupBy) > 0 || groupWaitSeconds > 0 || groupIntervalSeconds > 0 || repeatIntervalSeconds > 0 {
		return errors.New("grouping and repeat interval settings are not supported yet")
	}
	return nil
}
