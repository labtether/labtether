package jobqueue

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

type heartbeatProcessQueue struct {
	stubProcessQueue
	renew func(context.Context, string, string) error
}

func (q *heartbeatProcessQueue) leaseHeartbeatInterval() time.Duration { return 20 * time.Millisecond }

func (q *heartbeatProcessQueue) RenewClaim(ctx context.Context, id, token string) error {
	return q.renew(ctx, id, token)
}

// A temporary renewal failure must not abandon the lease while its actual
// handler is still running, allowing stale recovery to queue the same work.
func TestProcessOneRetainsHeartbeatAfterTransientFailure(t *testing.T) {
	for _, failure := range []string{"connection", "stalled-renewal"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			renewed := make(chan struct{})
			var attempts atomic.Int32
			q := &heartbeatProcessQueue{stubProcessQueue: stubProcessQueue{job: &Job{
				ID: "active-job", Kind: KindActionRun, LockToken: "original-claim",
			}}}
			q.renew = func(renewCtx context.Context, id, token string) error {
				if id != "active-job" || token != "original-claim" {
					return fmt.Errorf("renewed a different claim: %s/%s", id, token)
				}
				if attempts.Add(1) == 1 {
					if failure == "stalled-renewal" {
						<-renewCtx.Done()
						return renewCtx.Err()
					}
					return errors.New("temporary database connection failure")
				}
				select {
				case <-renewed:
				default:
					close(renewed)
				}
				return nil
			}
			w := NewWorker(nil)
			w.Register(KindActionRun, func(context.Context, *Job) error {
				select {
				case <-renewed:
					return nil
				case <-ctx.Done():
					return errors.New("heartbeat abandoned a running handler")
				}
			})
			if !w.processOneWithQueue(ctx, q) {
				t.Fatal("worker did not claim the job")
			}
			if q.completeCalls != 1 || q.failCalls != 0 || attempts.Load() < 2 {
				t.Fatalf("active claim not renewed: completed=%d failed=%d renewals=%d", q.completeCalls, q.failCalls, attempts.Load())
			}
		})
	}
}

func TestClaimHeartbeatStopsAfterOwnershipLoss(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprintf("committed_%v", committed), func(t *testing.T) {
			parent, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			var renewals atomic.Int32
			ready := make(chan struct{})
			job := &Job{ID: "lost-job", Kind: KindActionRun, LockToken: "old-owner"}
			q := &heartbeatProcessQueue{stubProcessQueue: stubProcessQueue{job: job}}
			q.renew = func(context.Context, string, string) error { <-ready; renewals.Add(1); return ErrClaimLost }
			w := NewWorker(nil)
			w.Register(KindActionRun, func(ctx context.Context, j *Job) error {
				// Model a successful transaction followed by a renewal that observes its
				// completed queue row. The worker must never fail/complete it a second time.
				if committed {
					j.Status = StatusCompleted
				}
				close(ready)
				<-ctx.Done()
				if !errors.Is(context.Cause(ctx), ErrClaimLost) {
					return fmt.Errorf("wrong cancellation cause: %v", context.Cause(ctx))
				}
				return ctx.Err()
			})
			if !w.processOneWithQueue(parent, q) {
				t.Fatal("job was not claimed")
			}
			if parent.Err() != nil || q.failCalls != 0 || q.completeCalls != 0 || renewals.Load() != 1 {
				t.Fatalf("lost claim retained work: parent=%v fail=%d complete=%d renew=%d", parent.Err(), q.failCalls, q.completeCalls, renewals.Load())
			}
		})
	}
}
