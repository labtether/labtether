package audit

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

// ReadCorrelationPage reads only a bounded projection of retained audit rows.
// It does not expose arbitrary Details, Reason, session or command fields.
// Source retention remains owned by Store.Append; this is not a history archive.
func (s *Store) ReadCorrelationPage(ctx context.Context, from, through time.Time, targets []string, after, upper string, started bool) ([]Event, string, bool, error) {
	if ctx == nil {
		return nil, upper, false, errors.New("audit page context required")
	}
	if err := ctx.Err(); err != nil {
		return nil, upper, false, err
	}
	if from.IsZero() || through.IsZero() || from.After(through) {
		return nil, upper, false, errors.New("invalid audit page window")
	}
	const pageSize = 128
	wanted := make(map[string]bool, len(targets))
	for _, id := range targets {
		wanted[id] = true
	}
	rows := make([]Event, 0, pageSize)
	// Bound lock admission too, without leaving a waiter goroutine behind.
	for {
		if err := ctx.Err(); err != nil {
			return nil, upper, false, err
		}
		if s.mu.TryRLock() {
			break
		}
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, upper, false, ctx.Err()
		case <-timer.C:
		}
	}
	defer s.mu.RUnlock()
	for _, event := range s.events {
		if err := ctx.Err(); err != nil {
			return nil, upper, false, err
		}
		if !wanted[event.Target] || event.Timestamp.Before(from) || event.Timestamp.After(through) {
			continue
		}
		if event.ID == "" || strings.TrimSpace(event.ID) != event.ID {
			return nil, upper, false, errors.New("invalid stored audit correlation identity")
		}
		if !started && event.ID > upper {
			upper = event.ID
		}
		if event.ID <= after || (started && event.ID > upper) {
			continue
		}
		i := sort.Search(len(rows), func(i int) bool { return rows[i].ID >= event.ID })
		if i < len(rows) && rows[i].ID == event.ID {
			continue
		}
		if i >= pageSize {
			continue
		}
		if len(rows) < pageSize {
			rows = append(rows, Event{})
		}
		copy(rows[i+1:], rows[i:])
		rows[i] = Event{ID: event.ID, Type: event.Type, ActorID: event.ActorID, Target: event.Target, Timestamp: event.Timestamp.UTC()}
	}
	if err := ctx.Err(); err != nil {
		return nil, upper, false, err
	}
	return rows, upper, len(rows) < pageSize, nil
}
