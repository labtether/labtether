package persistence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/incidents"
)

func TestMemoryAutoIncidentIsAtomicAndDeduplicated(t *testing.T) {
	s := NewMemoryIncidentStore()
	req := incidents.CreateIncidentRequest{Title: "Critical CPU", Severity: incidents.SeverityCritical}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Cancellation while the actual store lock is held must return without
	// writing either half or retaining a waiter that writes later.
	s.mu.Lock()
	waitCtx, stop := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() {
		_, _, err := s.CreateAutoIncidentForAlertInstanceContext(waitCtx, "canceled", req)
		result <- err
	}()
	stop()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			s.mu.Unlock()
			t.Fatalf("lock wait ignored cancellation: %v", err)
		}
	case <-ctx.Done():
		s.mu.Unlock()
		t.Fatal("canceled creator did not return while lock held")
	}
	if len(s.incidents) != 0 || len(s.links) != 0 {
		s.mu.Unlock()
		t.Fatal("canceled creator left incident or link")
	}
	s.mu.Unlock()

	// A manual incident linked to the same alert must not suppress automatic
	// creation. Simultaneous automatic creators must publish exactly one pair.
	manual, err := s.CreateIncident(incidents.CreateIncidentRequest{Title: "Manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.LinkIncidentAlert(manual.ID, incidents.LinkAlertRequest{AlertInstanceID: "instance", LinkType: incidents.LinkTypeRelated}); err != nil {
		t.Fatal(err)
	}
	type answer struct {
		incident incidents.Incident
		created  bool
		err      error
	}
	results := make(chan answer, 16)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := 0; i < cap(results); i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			inc, created, err := s.CreateAutoIncidentForAlertInstanceContext(ctx, " instance ", req)
			results <- answer{inc, created, err}
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	var createdID string
	for answer := range results {
		if answer.err != nil {
			t.Fatal(answer.err)
		}
		if answer.created {
			if createdID != "" || answer.incident.Source != incidents.SourceAlertAuto {
				t.Fatal("automatic creators produced duplicate or manual incident")
			}
			createdID = answer.incident.ID
		}
	}
	if createdID == "" || len(s.incidents) != 2 || len(s.links) != 2 {
		t.Fatalf("expected one manual and one automatic pair: created=%q incidents=%d links=%d", createdID, len(s.incidents), len(s.links))
	}
	links, err := s.ListIncidentAlertLinks(createdID, 10)
	if err != nil || len(links) != 1 || links[0].AlertInstanceID != "instance" || links[0].LinkType != incidents.LinkTypeTrigger {
		t.Fatalf("automatic pair is incomplete: %+v/%v", links, err)
	}
	if _, created, err := s.CreateAutoIncidentForAlertInstanceContext(ctx, "instance", req); err != nil || created {
		t.Fatalf("retry created a duplicate: %v/%v", created, err)
	}
}
