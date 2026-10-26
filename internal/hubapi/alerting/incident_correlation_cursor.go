package alerting

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/labtether/labtether/internal/incidents"
	"github.com/labtether/labtether/internal/persistence"
)

const incidentCorrelationSlice = 5 * time.Second

// Only IDs and cursors survive a tick. Each history page is released before
// another incident gets its turn. Restarting the process safely replays upserts.
type incidentCorrelationWork struct {
	discovery persistence.CorrelationCursor
	through   time.Time
	queue     list.List
	entries   map[string]*list.Element
}

type incidentCorrelationSweep struct {
	id       string
	identity [32]byte
	window   persistence.CorrelationWindow
	cursors  [4]persistence.CorrelationCursor
	done     [4]bool
	next     int
}

func (d *Deps) correlateOpenIncidents(parent context.Context) {
	if d.IncidentStore == nil || d.IncidentEventStore == nil || !d.incidentCorrelationMu.TryLock() {
		return
	}
	defer d.incidentCorrelationMu.Unlock()
	owner, ok := d.IncidentStore.(persistence.IncidentCorrelationPager)
	if !ok {
		log.Print("incident correlator: cursor owner unavailable")
		return
	}
	if _, ok := d.IncidentEventStore.(persistence.IncidentEventContextStore); !ok {
		log.Print("incident correlator: context event owner unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(parent, incidentCorrelationSlice)
	defer cancel()
	w := &d.incidentCorrelationWork
	if w.entries == nil {
		w.entries = make(map[string]*list.Element)
	}
	// Discovery has its own smaller share. A slow inventory must not starve
	// incidents already queued from an earlier successful page.
	lookup, endLookup := context.WithTimeout(ctx, time.Second)
	if err := w.discover(lookup, owner); err != nil && ctx.Err() == nil {
		log.Printf("incident correlator: discovery deferred: %v", err)
	}
	endLookup()
	finished := make(map[string]bool)
	idle := 0
	for ctx.Err() == nil && w.queue.Len() > 0 && idle < w.queue.Len() {
		element := w.queue.Front()
		sweep := element.Value.(*incidentCorrelationSweep)
		// Rotate before any read or write, including a cancellation or failure.
		w.queue.MoveToBack(element)
		if finished[sweep.id] {
			idle++
			continue
		}
		idle = 0
		current, found, err := owner.ReadIncidentCorrelation(ctx, sweep.id)
		if err != nil {
			finished[sweep.id] = true
			log.Printf("incident correlator: incident %s deferred: %v", sweep.id, err)
			continue
		}
		if !found || (current.Status != incidents.StatusOpen && current.Status != incidents.StatusInvestigating) {
			delete(w.entries, sweep.id)
			w.queue.Remove(element)
			continue
		}
		inc := correlationIncident(current)
		complete, err := d.correlateIncidentSlice(ctx, inc, sweep)
		if complete || err != nil {
			finished[sweep.id] = true
		}
		if err != nil && ctx.Err() == nil {
			log.Printf("incident correlator: incident %s deferred: %v", sweep.id, err)
		}
	}
}

func (w *incidentCorrelationWork) discover(ctx context.Context, owner persistence.IncidentCorrelationPager) error {
	if w.through.IsZero() {
		w.through = time.Now().UTC()
	}
	for ctx.Err() == nil {
		rows, next, done, err := owner.ReadActiveIncidentCorrelationPage(ctx, w.through, w.discovery)
		if err != nil {
			return err
		}
		if err := validateCorrelationPage(w.discovery, next, len(rows), done); err != nil {
			return err
		}
		w.discovery.Started, w.discovery.UpperID = next.Started, next.UpperID
		for _, row := range rows {
			if err := validateCorrelationRow(w.discovery, row.ID); err != nil {
				return err
			}
			if !row.OpenedAt.IsZero() && (row.Status == incidents.StatusOpen || row.Status == incidents.StatusInvestigating) {
				if _, exists := w.entries[row.ID]; !exists {
					w.entries[row.ID] = w.queue.PushBack(&incidentCorrelationSweep{id: row.ID})
				}
			}
			w.discovery.AfterID = row.ID
		}
		if done {
			w.discovery = persistence.CorrelationCursor{}
			w.through = time.Time{}
			return nil // Never begin a second discovery sweep in one tick.
		}
	}
	return ctx.Err()
}

func correlationIncident(row persistence.CorrelationIncident) incidents.Incident {
	return incidents.Incident{ID: row.ID, Status: row.Status, GroupID: row.GroupID,
		PrimaryAssetID: row.PrimaryAssetID, OpenedAt: row.OpenedAt}
}

func (d *Deps) correlateIncidentSlice(ctx context.Context, inc incidents.Incident, sweep *incidentCorrelationSweep) (bool, error) {
	if inc.ID != sweep.id || inc.OpenedAt.IsZero() {
		return false, fmt.Errorf("invalid incident cursor identity")
	}
	scope, err := d.incidentCorrelationScope(ctx, inc)
	if err != nil {
		return false, err
	}
	queryScope := scope.pageScope(inc.ID)
	identity, err := json.Marshal(struct {
		Scope persistence.IncidentCorrelationScope
		From  time.Time
	}{queryScope, inc.OpenedAt})
	if err != nil {
		return false, err
	}
	digest := sha256.Sum256(identity)
	if sweep.window.Through.IsZero() || sweep.identity != digest || sweep.complete() {
		*sweep = incidentCorrelationSweep{id: inc.ID, identity: digest,
			window: persistence.CorrelationWindow{From: inc.OpenedAt, Through: time.Now().UTC()}}
	}
	for i := 0; i < len(sweep.done); i++ {
		source := (sweep.next + i) % len(sweep.done)
		if sweep.done[source] {
			continue
		}
		sweep.next = (source + 1) % len(sweep.done)
		rows, next, done, err := d.readCorrelationPage(ctx, source, sweep.window, queryScope, sweep.cursors[source])
		if err != nil {
			return false, err
		}
		cursor := &sweep.cursors[source]
		if err := validateCorrelationPage(*cursor, next, len(rows), done); err != nil {
			return false, err
		}
		cursor.Started, cursor.UpperID = next.Started, next.UpperID
		for _, row := range rows {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if err := validateCorrelationRow(*cursor, row.id); err != nil {
				return false, err
			}
			if row.rejected {
				log.Printf("incident correlator: rejected malformed firing evidence %s", row.id)
			} else if err := d.correlateIncidentPage(ctx, inc, scope, row.data, sweep.window); err != nil {
				return false, err
			}
			// Failed effects never move this position. A retry replays only the
			// uncertain row, protected by the store's unique source-ref upsert.
			cursor.AfterID = row.id
		}
		sweep.done[source] = done
		return sweep.complete(), nil
	}
	return true, nil
}

func (s *incidentCorrelationSweep) complete() bool {
	for _, done := range s.done {
		if !done {
			return false
		}
	}
	return true
}

func validateCorrelationPage(current, next persistence.CorrelationCursor, count int, done bool) error {
	if !next.Started || count > persistence.CorrelationPageSize || (count == 0 && !done) ||
		(current.Started && current.UpperID != next.UpperID) || next.UpperID < current.AfterID {
		return fmt.Errorf("invalid incident correlation page")
	}
	return nil
}

func validateCorrelationRow(cursor persistence.CorrelationCursor, id string) error {
	if !correlationID(id) || id <= cursor.AfterID || id > cursor.UpperID {
		return fmt.Errorf("invalid incident correlation row order")
	}
	return nil
}
