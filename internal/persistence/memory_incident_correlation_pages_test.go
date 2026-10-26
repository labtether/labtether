package persistence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/actions"
)

func TestMemoryIncidentCorrelationPagesFilterAndResume(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	f := newCorrelationPageFixture("memory-pages-")
	is, as, us, als, aus := NewMemoryIncidentStore(), NewMemoryActionStore(), NewMemoryUpdateStore(), NewMemoryAlertInstanceStore(), NewMemoryAuditStore()
	// Seed persisted historic rows, including malformed legacy evidence. The
	// assertions exercise the production readers, not replacement page stubs.
	for _, row := range f.incidents {
		is.incidents[row.ID] = row
	}
	for _, row := range f.actions {
		as.runs[row.ID] = row
	}
	for _, row := range f.updates {
		us.runs[row.ID] = row
	}
	for _, row := range f.alerts {
		als.instances[row.ID] = row
	}
	for _, row := range f.audits {
		if err := aus.Append(row); err != nil {
			t.Fatal(err)
		}
	}
	owners := correlationPageOwners{is, as, us, als, aus}
	checkCorrelationPageContract(t, ctx, f, owners, func() {
		as.mu.Lock()
		defer as.mu.Unlock()
		row := as.runs[f.actions[1].ID]
		row.UpdatedAt = f.at.Add(time.Hour)
		as.runs[row.ID] = row
		late := actions.Run{ID: f.prefix + "zz-late", Type: "command", Status: "failed", Target: f.scope.AssetIDs[0], CreatedAt: f.at}
		as.runs[late.ID] = late
	})
	for _, item := range []struct {
		name string
		mu   *sync.RWMutex
		read func(context.Context) error
	}{
		{"incidents", &is.mu, func(c context.Context) error {
			_, _, _, err := is.ReadActiveIncidentCorrelationPage(c, f.window.Through, CorrelationCursor{})
			return err
		}},
		{"incident exact", &is.mu, func(c context.Context) error {
			_, _, err := is.ReadIncidentCorrelation(c, f.scope.IncidentID)
			return err
		}},
		{"actions", &as.mu, func(c context.Context) error {
			_, _, _, err := as.ReadActionCorrelationPage(c, f.window, f.scope, CorrelationCursor{})
			return err
		}},
		{"updates", &us.mu, func(c context.Context) error {
			_, _, _, err := us.ReadUpdateCorrelationPage(c, f.window, f.scope, CorrelationCursor{})
			return err
		}},
		{"alerts", &als.mu, func(c context.Context) error {
			_, _, _, err := als.ReadAlertCorrelationPage(c, f.window, f.scope, CorrelationCursor{})
			return err
		}},
	} {
		t.Run(item.name, func(t *testing.T) {
			item.mu.Lock()
			defer item.mu.Unlock()
			short, stop := context.WithTimeout(ctx, 10*time.Millisecond)
			defer stop()
			if err := item.read(short); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("writer wait lost context: %v", err)
			}
		})
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if _, _, _, err := aus.ReadAuditCorrelationPage(canceled, f.window, f.scope, CorrelationCursor{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("audit ignored cancellation: %v", err)
	}
}
