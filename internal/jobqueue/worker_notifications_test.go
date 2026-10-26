package jobqueue

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNotificationLoopStopsOnBrokenConnection(t *testing.T) {
	broken := errors.New("connection closed")
	claims, waits := 0, 0
	err := runNotificationLoop(context.Background(), time.Second, func(context.Context) bool {
		claims++
		return false
	}, func(context.Context) error {
		waits++
		if waits > 1 {
			t.Fatal("retried a broken listener instead of returning to bounded polling")
		}
		return broken
	})
	if !errors.Is(err, broken) || claims != 1 || waits != 1 {
		t.Fatalf("err=%v claims=%d waits=%d", err, claims, waits)
	}
}

func TestNotificationLoopDrainsWorkOnNotificationAndDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	claims, waits := 0, 0
	err := runNotificationLoop(ctx, time.Second, func(context.Context) bool {
		claims++
		return claims%3 != 0
	}, func(waitCtx context.Context) error {
		if _, ok := waitCtx.Deadline(); !ok {
			t.Fatal("notification wait has no polling deadline")
		}
		waits++
		switch waits {
		case 1:
			return nil
		case 2:
			return context.DeadlineExceeded
		default:
			cancel()
			return ctx.Err()
		}
	})
	if !errors.Is(err, context.Canceled) || claims != 9 || waits != 3 {
		t.Fatalf("err=%v claims=%d waits=%d", err, claims, waits)
	}
}
