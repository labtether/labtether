package shared

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
)

type DeadLetterEventResponse struct {
	ID         string    `json:"id"`
	Component  string    `json:"component"`
	Subject    string    `json:"subject"`
	Deliveries uint64    `json:"deliveries"`
	Error      string    `json:"error"`
	PayloadB64 string    `json:"payload_b64,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type DeadLetterTopEntry struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type DeadLetterTrendPoint struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Count int       `json:"count"`
}

type DeadLetterAnalyticsResponse struct {
	Window          string                 `json:"window"`
	Bucket          string                 `json:"bucket"`
	Total           int                    `json:"total"`
	RatePerHour     float64                `json:"rate_per_hour"`
	RatePerDay      float64                `json:"rate_per_day"`
	Trend           []DeadLetterTrendPoint `json:"trend"`
	TopComponents   []DeadLetterTopEntry   `json:"top_components"`
	TopSubjects     []DeadLetterTopEntry   `json:"top_subjects"`
	TopErrorClasses []DeadLetterTopEntry   `json:"top_error_classes"`
}

func MapLogEventToDeadLetter(event logs.Event) DeadLetterEventResponse {
	fields := event.Fields
	if fields == nil {
		fields = map[string]string{}
	}

	deliveries, _ := strconv.ParseUint(strings.TrimSpace(fields["deliveries"]), 10, 64)
	id := strings.TrimSpace(fields["event_id"])
	if id == "" {
		id = event.ID
	}

	errMessage := strings.TrimSpace(fields["error"])
	if errMessage == "" {
		errMessage = strings.TrimSpace(event.Message)
	}

	return DeadLetterEventResponse{
		ID:         id,
		Component:  strings.TrimSpace(fields["component"]),
		Subject:    strings.TrimSpace(fields["subject"]),
		Deliveries: deliveries,
		Error:      errMessage,
		PayloadB64: strings.TrimSpace(fields["payload_b64"]),
		CreatedAt:  event.Timestamp.UTC(),
	}
}

func MapProjectedDeadLetterEvent(event logs.DeadLetterEvent) DeadLetterEventResponse {
	return DeadLetterEventResponse{
		ID:         strings.TrimSpace(event.ID),
		Component:  strings.TrimSpace(event.Component),
		Subject:    strings.TrimSpace(event.Subject),
		Deliveries: event.Deliveries,
		Error:      strings.TrimSpace(event.Error),
		PayloadB64: strings.TrimSpace(event.PayloadB64),
		CreatedAt:  event.CreatedAt.UTC(),
	}
}

func QueryDeadLetterEventResponses(
	store persistence.LogStore,
	from time.Time,
	to time.Time,
	limit int,
) ([]DeadLetterEventResponse, error) {
	if optimizedStore, ok := store.(persistence.DeadLetterLogStore); ok {
		projected, err := optimizedStore.QueryDeadLetterEvents(from, to, limit)
		if err != nil {
			return nil, err
		}
		out := make([]DeadLetterEventResponse, 0, len(projected))
		for _, event := range projected {
			out = append(out, MapProjectedDeadLetterEvent(event))
		}
		return out, nil
	}

	events, err := store.QueryEvents(logs.QueryRequest{
		Source: "dead_letter",
		Level:  "error",
		From:   from,
		To:     to,
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]DeadLetterEventResponse, 0, len(events))
	for _, event := range events {
		out = append(out, MapLogEventToDeadLetter(event))
	}
	return out, nil
}

func CountDeadLetterEvents(
	store persistence.LogStore,
	from time.Time,
	to time.Time,
) (int, error) {
	if counter, ok := store.(persistence.DeadLetterLogCountStore); ok {
		return counter.CountDeadLetterEvents(from, to)
	}
	// Fallback for stores without optimized counts keeps legacy behavior where
	// totals are derived from fetched events.
	events, err := QueryDeadLetterEventResponses(store, from, to, 1000)
	if err != nil {
		return 0, err
	}
	return len(events), nil
}

func BuildDeadLetterAnalytics(events []DeadLetterEventResponse, from, to time.Time, window time.Duration) DeadLetterAnalyticsResponse {
	bucketSize := ChooseDeadLetterBucket(window)
	if !to.After(from) {
		to = from.Add(bucketSize)
	}

	total := len(events)
	componentCounts := make(map[string]int, 16)
	subjectCounts := make(map[string]int, 16)
	errorClassCounts := make(map[string]int, 16)
	bucketCounts := make(map[int64]int, 64)

	for _, event := range events {
		component := strings.TrimSpace(event.Component)
		if component == "" {
			component = "unknown"
		}
		subject := strings.TrimSpace(event.Subject)
		if subject == "" {
			subject = "unknown"
		}
		errClass := ClassifyDeadLetterError(event.Error)

		componentCounts[component]++
		subjectCounts[subject]++
		errorClassCounts[errClass]++

		eventTime := event.CreatedAt.UTC()
		if eventTime.Before(from) || eventTime.After(to) {
			continue
		}
		bucketStart := eventTime.Truncate(bucketSize)
		bucketCounts[bucketStart.Unix()]++
	}

	trend := make([]DeadLetterTrendPoint, 0, 64)
	for bucketStart := from.Truncate(bucketSize); !bucketStart.After(to); bucketStart = bucketStart.Add(bucketSize) {
		next := bucketStart.Add(bucketSize)
		if next.After(to) {
			next = to
		}
		trend = append(trend, DeadLetterTrendPoint{
			Start: bucketStart,
			End:   next,
			Count: bucketCounts[bucketStart.Unix()],
		})
	}

	hours := window.Hours()
	ratePerHour, ratePerDay := DeadLetterRates(total, hours)

	return DeadLetterAnalyticsResponse{
		Window:          window.String(),
		Bucket:          bucketSize.String(),
		Total:           total,
		RatePerHour:     ratePerHour,
		RatePerDay:      ratePerDay,
		Trend:           trend,
		TopComponents:   TopDeadLetterEntries(componentCounts, 5),
		TopSubjects:     TopDeadLetterEntries(subjectCounts, 5),
		TopErrorClasses: TopDeadLetterEntries(errorClassCounts, 5),
	}
}

func DeadLetterAnalyticsWithTotal(
	analytics DeadLetterAnalyticsResponse,
	total int,
	window time.Duration,
) DeadLetterAnalyticsResponse {
	if total < 0 {
		total = 0
	}
	analytics.Total = total
	ratePerHour, ratePerDay := DeadLetterRates(total, window.Hours())
	analytics.RatePerHour = ratePerHour
	analytics.RatePerDay = ratePerDay
	return analytics
}

func DeadLetterRates(total int, windowHours float64) (ratePerHour float64, ratePerDay float64) {
	if windowHours <= 0 {
		return 0, 0
	}
	ratePerHour = float64(total) / windowHours
	days := windowHours / 24
	if days > 0 {
		ratePerDay = float64(total) / days
	}
	return ratePerHour, ratePerDay
}

func ChooseDeadLetterBucket(window time.Duration) time.Duration {
	switch {
	case window <= 2*time.Hour:
		return 5 * time.Minute
	case window <= 24*time.Hour:
		return time.Hour
	case window <= 7*24*time.Hour:
		return 6 * time.Hour
	default:
		return 24 * time.Hour
	}
}

func TopDeadLetterEntries(counts map[string]int, limit int) []DeadLetterTopEntry {
	if len(counts) == 0 {
		return []DeadLetterTopEntry{}
	}

	out := make([]DeadLetterTopEntry, 0, len(counts))
	for key, count := range counts {
		out = append(out, DeadLetterTopEntry{Key: key, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Key < out[j].Key
		}
		return out[i].Count > out[j].Count
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func ClassifyDeadLetterError(message string) string {
	text := strings.ToLower(strings.TrimSpace(message))
	switch {
	case text == "":
		return "unknown"
	case strings.Contains(text, "timeout"),
		strings.Contains(text, "timed out"),
		strings.Contains(text, "deadline"):
		return "timeout"
	case strings.Contains(text, "unauthorized"),
		strings.Contains(text, "forbidden"),
		strings.Contains(text, "auth"):
		return "auth"
	case strings.Contains(text, "decode"),
		strings.Contains(text, "unmarshal"),
		strings.Contains(text, "parse"),
		strings.Contains(text, "invalid json"):
		return "decode"
	case strings.Contains(text, "dial"),
		strings.Contains(text, "connection"),
		strings.Contains(text, "network"),
		strings.Contains(text, "refused"),
		strings.Contains(text, "unreachable"):
		return "network"
	case strings.Contains(text, "not found"):
		return "not_found"
	case strings.Contains(text, "permission"):
		return "permission"
	case strings.Contains(text, "invalid"),
		strings.Contains(text, "bad request"),
		strings.Contains(text, "validation"):
		return "validation"
	default:
		return "other"
	}
}
