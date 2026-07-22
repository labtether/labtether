package schedules

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

const MaxCronExpressionLength = 256

var cronParser = cron.NewParser(
	cron.Minute |
		cron.Hour |
		cron.Dom |
		cron.Month |
		cron.Dow |
		cron.Descriptor,
)

// NextRun parses the supported five-field/descriptor cron syntax and returns
// the next UTC occurrence strictly after the supplied time.
func NextRun(expression string, after time.Time) (time.Time, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return time.Time{}, fmt.Errorf("cron expression is required")
	}
	if len(expression) > MaxCronExpressionLength {
		return time.Time{}, fmt.Errorf("cron expression exceeds maximum length")
	}
	scheduleExpression := expression
	if strings.HasPrefix(scheduleExpression, "TZ=") || strings.HasPrefix(scheduleExpression, "CRON_TZ=") {
		separator := strings.IndexByte(scheduleExpression, ' ')
		equals := strings.IndexByte(scheduleExpression, '=')
		if separator <= equals+1 || separator == len(scheduleExpression)-1 {
			return time.Time{}, fmt.Errorf("timezone-prefixed cron requires a timezone and schedule")
		}
		scheduleExpression = strings.TrimSpace(scheduleExpression[separator+1:])
		if scheduleExpression == "" {
			return time.Time{}, fmt.Errorf("timezone-prefixed cron requires a schedule")
		}
	}
	if strings.HasPrefix(scheduleExpression, "@every") {
		return time.Time{}, fmt.Errorf("@every schedules are unsupported; use minute-granularity cron syntax")
	}
	parseExpression := expression
	if !strings.HasPrefix(expression, "TZ=") && !strings.HasPrefix(expression, "CRON_TZ=") {
		// robfig/cron otherwise inherits the process-local timezone. Force UTC so
		// two hub replicas cannot calculate different occurrences; callers can
		// opt into an IANA timezone with a CRON_TZ= prefix.
		parseExpression = "CRON_TZ=UTC " + expression
	}
	schedule, err := cronParser.Parse(parseExpression)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron expression: %w", err)
	}
	next := schedule.Next(after.UTC()).UTC()
	if next.IsZero() || !next.After(after.UTC()) {
		return time.Time{}, fmt.Errorf("cron expression has no future occurrence")
	}
	return next, nil
}
