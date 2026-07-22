package persistence

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestPostgresListFileTransfersActorScopeFilterOrderAndPagination(t *testing.T) {
	store := newTestPostgresStore(t)
	suffix := time.Now().UTC().UnixNano()
	actorA := fmt.Sprintf("file-transfer-list-a-%d", suffix)
	actorB := fmt.Sprintf("file-transfer-list-b-%d", suffix)
	t.Cleanup(func() {
		_, _ = store.pool.Exec(context.Background(), `DELETE FROM file_transfers WHERE actor_id IN ($1, $2)`, actorA, actorB)
	})

	create := func(actorID, status, name string) FileTransfer {
		t.Helper()
		transfer := FileTransfer{
			ActorID:    actorID,
			SourceType: "connection",
			SourceID:   "source",
			SourcePath: "/source/" + name,
			DestType:   "connection",
			DestID:     "destination",
			DestPath:   "/destination/" + name,
			FileName:   name,
			Status:     status,
		}
		if err := store.CreateFileTransfer(context.Background(), &transfer); err != nil {
			t.Fatalf("CreateFileTransfer(%s): %v", name, err)
		}
		return transfer
	}

	oldestA := create(actorA, "completed", "oldest")
	middleA := create(actorA, "pending", "middle")
	otherActor := create(actorB, "completed", "private")
	newestA := create(actorA, "completed", "newest")

	firstPage, total, err := store.ListFileTransfers(context.Background(), actorA, "", 2, 0)
	if err != nil {
		t.Fatalf("ListFileTransfers first page: %v", err)
	}
	if total != 3 {
		t.Fatalf("total=%d, want 3 actor-a rows", total)
	}
	if len(firstPage) != 2 || firstPage[0].ID != newestA.ID || firstPage[1].ID != middleA.ID {
		t.Fatalf("first page IDs=%v, want newest then middle", fileTransferIDs(firstPage))
	}
	for _, transfer := range firstPage {
		if transfer.ActorID != actorA || transfer.ID == otherActor.ID {
			t.Fatalf("cross-actor row disclosed: %+v", transfer)
		}
	}

	secondPage, total, err := store.ListFileTransfers(context.Background(), actorA, "", 2, 2)
	if err != nil {
		t.Fatalf("ListFileTransfers second page: %v", err)
	}
	if total != 3 || len(secondPage) != 1 || secondPage[0].ID != oldestA.ID {
		t.Fatalf("second page total=%d IDs=%v, want oldest only", total, fileTransferIDs(secondPage))
	}

	completed, total, err := store.ListFileTransfers(context.Background(), actorA, "completed", 10, 0)
	if err != nil {
		t.Fatalf("ListFileTransfers completed: %v", err)
	}
	if total != 2 || len(completed) != 2 || completed[0].ID != newestA.ID || completed[1].ID != oldestA.ID {
		t.Fatalf("completed total=%d IDs=%v", total, fileTransferIDs(completed))
	}

	actorBRows, total, err := store.ListFileTransfers(context.Background(), actorB, "", 10, 0)
	if err != nil {
		t.Fatalf("ListFileTransfers actor-b: %v", err)
	}
	if total != 1 || len(actorBRows) != 1 || actorBRows[0].ID != otherActor.ID {
		t.Fatalf("actor-b total=%d IDs=%v", total, fileTransferIDs(actorBRows))
	}
}

func TestPostgresListFileTransfersHonorsContextCancellation(t *testing.T) {
	store := newTestPostgresStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := store.ListFileTransfers(ctx, "actor", "", 10, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want context.Canceled", err)
	}
}

func fileTransferIDs(transfers []FileTransfer) []string {
	ids := make([]string, len(transfers))
	for i := range transfers {
		ids[i] = transfers[i].ID
	}
	return ids
}
