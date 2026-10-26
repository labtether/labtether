package alerting

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/alerts"
	"github.com/labtether/labtether/internal/groupmaintenance"
	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/notifications"
	"github.com/labtether/labtether/internal/persistence"
)

func (s *notificationSecurityStore) GetNotificationChannelContext(ctx context.Context, id string) (notifications.Channel, bool, error) {
	if err := ctx.Err(); err != nil {
		return notifications.Channel{}, false, err
	}
	return s.GetNotificationChannel(id)
}

func (s *notificationSecurityStore) CompareAndSwapNotificationChannelConfigContext(ctx context.Context, id string, expected, replacement map[string]any) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return s.CompareAndSwapNotificationChannelConfig(id, expected, replacement)
}

func (s *unavailableSilenceStore) ListAlertSilencesContext(ctx context.Context, limit int, active bool) ([]alerts.AlertSilence, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.ListAlertSilences(limit, active)
}

func digestAllowGuardrails(ctx context.Context, _ string, _ time.Time) (groupfeatures.GroupMaintenanceGuardrails, error) {
	return groupfeatures.GroupMaintenanceGuardrails{}, ctx.Err()
}

type digestChannelBoundary struct {
	*notificationSecurityStore
	get func(context.Context)
	cas func(context.Context)
}

func (s *digestChannelBoundary) GetNotificationChannelContext(ctx context.Context, id string) (notifications.Channel, bool, error) {
	if s.get != nil {
		s.get(ctx)
	}
	return s.notificationSecurityStore.GetNotificationChannelContext(ctx, id)
}

func (s *digestChannelBoundary) CompareAndSwapNotificationChannelConfigContext(ctx context.Context, id string, expected, replacement map[string]any) (bool, error) {
	if s.cas != nil {
		s.cas(ctx)
	}
	return s.notificationSecurityStore.CompareAndSwapNotificationChannelConfigContext(ctx, id, expected, replacement)
}

type digestSendDeadline struct {
	*apnsFanoutAdapterStub
	check func(context.Context)
}

func (a *digestSendDeadline) Send(ctx context.Context, config, payload map[string]any) error {
	a.check(ctx)
	return a.apnsFanoutAdapterStub.Send(ctx, config, payload)
}

type digestMaintenanceContextStore struct {
	persistence.GroupMaintenanceStore
	read func(context.Context) ([]groupmaintenance.MaintenanceWindow, error)
}

func (s *digestMaintenanceContextStore) ListGroupMaintenanceWindowsContext(ctx context.Context, _ string, _ *time.Time, _ int) ([]groupmaintenance.MaintenanceWindow, error) {
	return s.read(ctx)
}

// Exercise the real worker without sleeps. The clock moves at actual store
// boundaries, where queued claims and slow reads can lose their send reserve.
func TestPushDigestLeaseAndPolicyBoundsBeforeDelivery(t *testing.T) {
	for _, kind := range []string{"expired", "reserve", "slow_policy", "expired_policy", "canceled_policy", "canceled_channel", "slow_channel", "repeated_groups"} {
		t.Run(kind, func(t *testing.T) {
			d, store, adapter, claim := digestSuppressionFixture(t)
			now := time.Now().UTC()
			claim.LeaseExpiresAt = now.Add(pushDigestLeaseDuration)
			d.pushDigestClock = func() time.Time { return now }
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reads := 0
			claim.Events = []persistence.PushDigestEvent{claim.Events[0]}
			maintenance := &groupfeatures.Deps{GroupMaintenanceStore: &digestMaintenanceContextStore{read: func(readCtx context.Context) ([]groupmaintenance.MaintenanceWindow, error) {
				reads++
				if deadline, ok := readCtx.Deadline(); !ok || time.Until(deadline) > pushDigestPolicyBudget {
					t.Error("unbounded policy context")
				}
				switch kind {
				case "slow_policy":
					now = now.Add(6 * time.Second)
				case "expired_policy":
					now = claim.LeaseExpiresAt
				case "canceled_policy":
					cancel()
				}
				return nil, readCtx.Err()
			}}}
			d.EvaluateGuardrailsContext = maintenance.EvaluateGuardrailsContext
			switch kind {
			case "expired":
				claim.LeaseExpiresAt = now.Add(-time.Second)
			case "reserve":
				claim.LeaseExpiresAt = now.Add(notificationDispatchTimeout + pushDigestSettleBudget - time.Second)
			case "canceled_channel", "slow_channel":
				base := d.NotificationStore.(*notificationSecurityStore)
				d.NotificationStore = &digestChannelBoundary{notificationSecurityStore: base, get: func(readCtx context.Context) {
					if _, ok := readCtx.Deadline(); !ok {
						t.Error("channel context has no deadline")
					}
					if kind == "canceled_channel" {
						cancel()
					} else {
						now = claim.LeaseExpiresAt
					}
				}}
			case "repeated_groups":
				d.NotificationAdapters[notifications.ChannelTypeAPNs] = &digestSendDeadline{apnsFanoutAdapterStub: adapter, check: func(sendCtx context.Context) {
					deadline, ok := sendCtx.Deadline()
					if !ok || time.Until(deadline) > notificationDispatchTimeout || deadline.After(claim.LeaseExpiresAt.Add(-pushDigestSettleBudget)) {
						t.Error("send exceeds lease or loses settlement reserve")
					}
				}}
				for i := 1; i < persistence.MaxPushDigestEventsPerDevice; i++ {
					event := claim.Events[0]
					event.ID = fmt.Sprintf("entry-%d", i)
					claim.Events = append(claim.Events, event)
				}
			}
			store.claims = []persistence.PushDigestClaim{claim}
			d.ProcessDuePushDigests(ctx)
			if kind == "repeated_groups" {
				if reads != 1 || len(adapter.calls) != 1 || adapter.calls[0].payload["digest_count"] != 128 || len(store.settlements) != 1 || len(store.settlements[0].Sent) != 128 {
					t.Fatalf("shared policy/send: reads=%d sends=%d settlements=%+v", reads, len(adapter.calls), store.settlements)
				}
				return
			}
			if len(adapter.calls) != 0 {
				t.Fatal("expired, canceled or stale policy sent APNs")
			}
			for _, settled := range store.settlements {
				if len(settled.RetryAt)+len(settled.Discarded)+len(settled.Sent) != 0 || len(settled.Deferred) != 1 {
					t.Fatalf("unsent claim spent attempt or lost entry: %+v", settled)
				}
			}
			if kind == "slow_policy" || kind == "reserve" {
				if len(store.settlements) != 1 {
					t.Fatal("valid lease did not defer unsent work")
				}
			} else if len(store.settlements) != 0 {
				t.Fatal("canceled/expired claim changed owned state")
			}
		})
	}
}

func TestPushDigestCredentialCASUsesClaimContext(t *testing.T) {
	d, store, adapter, claim := digestSuppressionFixture(t)
	d.NotificationSecrets = testutil.TestSecretsManager(t)
	base := d.NotificationStore.(*notificationSecurityStore)
	channel := digestChannel()
	channel.Config["auth_key_path"] = "synthetic-path-not-read"
	base.seed(channel)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := false
	d.NotificationStore = &digestChannelBoundary{notificationSecurityStore: base, cas: func(writeCtx context.Context) {
		called = true
		if _, ok := writeCtx.Deadline(); !ok {
			t.Error("credential CAS has no deadline")
		}
		cancel()
	}}
	store.claims = []persistence.PushDigestClaim{claim}
	d.ProcessDuePushDigests(ctx)
	if !called || base.migrations() != 0 || len(adapter.calls) != 0 || len(store.settlements) != 0 {
		t.Fatal("canceled credential migration changed state or sent")
	}
}
