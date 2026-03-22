package shared

import (
	"net/http"
	"time"

	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/servicehttp"
)

// DeadLetterDeps holds the dependencies needed by the dead-letter queue handler.
type DeadLetterDeps struct {
	LogStore persistence.LogStore
}

// HandleDeadLetters handles GET /queue/dead-letters.
func (d *DeadLetterDeps) HandleDeadLetters(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/queue/dead-letters" {
		servicehttp.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	window := ParseDurationParam(r.URL.Query().Get("window"), 24*time.Hour, time.Minute, 30*24*time.Hour)
	to := ParseTimestampParam(r.URL.Query().Get("to"), time.Now().UTC())
	from := ParseTimestampParam(r.URL.Query().Get("from"), to.Add(-window))
	listLimit := ParseLimit(r, 50)
	queryLimit := listLimit
	if queryLimit < 1000 {
		queryLimit = 1000
	}

	deadLetters, err := QueryDeadLetterEventResponses(d.LogStore, from, to, queryLimit)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to query dead-letter events")
		return
	}

	listedDeadLetters := deadLetters
	if len(listedDeadLetters) > listLimit {
		listedDeadLetters = listedDeadLetters[:listLimit]
	}

	analytics := BuildDeadLetterAnalytics(deadLetters, from, to, window)
	total := analytics.Total
	if counted, countErr := CountDeadLetterEvents(d.LogStore, from, to); countErr != nil {
		if len(listedDeadLetters) > total {
			total = len(listedDeadLetters)
		}
	} else {
		if counted > total {
			total = counted
		}
		if len(listedDeadLetters) > total {
			total = len(listedDeadLetters)
		}
	}
	analytics = DeadLetterAnalyticsWithTotal(analytics, total, window)

	servicehttp.WriteJSON(w, http.StatusOK, map[string]any{
		"from":      from,
		"to":        to,
		"window":    window.String(),
		"events":    listedDeadLetters,
		"listed":    len(listedDeadLetters),
		"total":     total,
		"analytics": analytics,
	})
}
