package resources

import (
	"context"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/persistence"
)

// panicOnceTransferStore panics on the first load, as a buggy protocol client
// or store driver might inside the background transfer goroutine.
type panicOnceTransferStore struct {
	*testFileTransferStore
	once sync.Once
}

func (s *panicOnceTransferStore) GetFileTransfer(ctx context.Context, id string) (*persistence.FileTransfer, error) {
	s.once.Do(func() { panic("protocol client bug") })
	return s.testFileTransferStore.GetFileTransfer(ctx, id)
}

// A panic in one transfer goroutine used to terminate the whole Hub.
func TestFileTransferPanicFailsTransferInsteadOfCrashingHub(t *testing.T) {
	store := &panicOnceTransferStore{testFileTransferStore: newTestFileTransferStore(&persistence.FileTransfer{
		ID: "ft-panic", ActorID: "op-1", Status: "pending",
	})}
	active := &sync.Map{}
	d := &Deps{FileTransferStore: store, ActiveTransfers: active}
	ctx, cancel := context.WithCancel(context.Background())
	active.Store("ft-panic", cancel)

	d.runFileTransferSafely(ctx, cancel, "ft-panic", fileTransferStartRequest{})

	ft, err := store.testFileTransferStore.GetFileTransfer(context.Background(), "ft-panic")
	if err != nil {
		t.Fatal(err)
	}
	if ft.Status != "failed" || ft.Error == nil || *ft.Error != transferPanicMessage || ft.CompletedAt == nil {
		t.Fatalf("panicked transfer = %+v", ft)
	}
	if _, ok := active.Load("ft-panic"); ok {
		t.Fatal("panicked transfer stayed registered as active")
	}
	if ctx.Err() == nil {
		t.Fatal("panicked transfer context was not cancelled")
	}
}

type activeTransferStore struct {
	*testFileTransferStore
}

func (s activeTransferStore) ListActiveFileTransfers(context.Context) ([]persistence.FileTransfer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]persistence.FileTransfer, 0, len(s.transfers))
	for _, ft := range s.transfers {
		if ft.Status == "pending" || ft.Status == "in_progress" {
			out = append(out, *ft)
		}
	}
	return out, nil
}

// Transfers from a previous Hub process can never finish; they must not stay
// "in progress" forever after a restart.
func TestFailInterruptedFileTransfersMarksOnlyActiveTransfers(t *testing.T) {
	store := activeTransferStore{newTestFileTransferStore(
		&persistence.FileTransfer{ID: "ft-pending", Status: "pending"},
		&persistence.FileTransfer{ID: "ft-running", Status: "in_progress"},
		&persistence.FileTransfer{ID: "ft-done", Status: "completed"},
	)}
	marked, err := FailInterruptedFileTransfers(context.Background(), store)
	if err != nil || marked != 2 {
		t.Fatalf("marked=%d err=%v, want 2", marked, err)
	}
	for id, want := range map[string]string{"ft-pending": "failed", "ft-running": "failed", "ft-done": "completed"} {
		ft, err := store.GetFileTransfer(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if ft.Status != want {
			t.Fatalf("%s status = %q, want %q", id, ft.Status, want)
		}
	}
}
