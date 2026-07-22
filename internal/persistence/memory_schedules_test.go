package persistence

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/labtether/labtether/internal/schedules"
)

func TestMemoryScheduleStoreEnforcesConcurrentPrincipalCapacityAndRecoversAfterDelete(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryScheduleStore()
	for i := 0; i < schedules.MaxScheduledTasksPerPrincipal-1; i++ {
		if err := store.CreateScheduledTask(ctx, capacityTestSchedule(i, "principal-1")); err != nil {
			t.Fatalf("prefill %d: %v", i, err)
		}
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var creates sync.WaitGroup
	creates.Add(2)
	for i := 0; i < 2; i++ {
		i := i
		go func() {
			defer creates.Done()
			<-start
			errs <- store.CreateScheduledTask(ctx, capacityTestSchedule(10_000+i, "principal-1"))
		}()
	}
	close(start)
	creates.Wait()
	close(errs)
	successes, capacityErrors := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, schedules.ErrScheduledTaskCapacityExceeded):
			capacityErrors++
		default:
			t.Fatalf("unexpected concurrent create error: %v", err)
		}
	}
	if successes != 1 || capacityErrors != 1 {
		t.Fatalf("successes=%d capacityErrors=%d, want 1/1", successes, capacityErrors)
	}
	if err := store.CreateScheduledTask(ctx, capacityTestSchedule(20_000, "principal-1")); !errors.Is(err, schedules.ErrScheduledTaskCapacityExceeded) {
		t.Fatalf("over-cap create error=%v", err)
	}
	_, total, err := store.ListScheduledTasks(ctx, schedules.MaxScheduledTasksGlobal, 0)
	if err != nil || total != schedules.MaxScheduledTasksPerPrincipal {
		t.Fatalf("exact-cap list total=%d err=%v", total, err)
	}
	if err := store.DeleteScheduledTask(ctx, "schedule-0"); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateScheduledTask(ctx, capacityTestSchedule(30_000, "principal-1")); err != nil {
		t.Fatalf("create after delete: %v", err)
	}
}

func TestMemoryScheduleStoreEnforcesGlobalCapacity(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryScheduleStore()
	for i := 0; i < schedules.MaxScheduledTasksGlobal; i++ {
		principal := fmt.Sprintf("principal-%d", i/schedules.MaxScheduledTasksPerPrincipal)
		if err := store.CreateScheduledTask(ctx, capacityTestSchedule(i, principal)); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	if err := store.CreateScheduledTask(ctx, capacityTestSchedule(schedules.MaxScheduledTasksGlobal+1, "unused-principal")); !errors.Is(err, schedules.ErrScheduledTaskCapacityExceeded) {
		t.Fatalf("global over-cap create error=%v", err)
	}
	page, total, err := store.ListScheduledTasks(ctx, schedules.MaxScheduledTaskPageSize, 0)
	if err != nil || total != schedules.MaxScheduledTasksGlobal || len(page) != schedules.MaxScheduledTaskPageSize {
		t.Fatalf("bounded page len=%d total=%d err=%v", len(page), total, err)
	}
}

func capacityTestSchedule(index int, principal string) schedules.ScheduledTask {
	return schedules.ScheduledTask{
		ID: fmt.Sprintf("schedule-%d", index), Name: "Capacity", CronExpr: "@hourly", Command: "uptime",
		Targets: []string{"asset-1"}, CreatedBy: principal,
	}
}
