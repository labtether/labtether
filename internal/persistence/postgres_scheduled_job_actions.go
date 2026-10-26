package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/jobqueue"
)

// Each scheduled child receives its own durable pre-dispatch record while the
// occurrence's queue claim is locked. No child is created after claim loss.
func (s *PostgresStore) StartScheduledAction(parent context.Context, claim *jobqueue.Job, target string) (actions.Job, error) {
	ctx, cancel := context.WithTimeout(parent, jobExecutionTimeout)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return actions.Job{}, err
	}
	defer rollbackJobExecution(tx)
	if err = lockJobClaim(ctx, tx, claim, false); err != nil {
		return actions.Job{}, err
	}
	owner, err := lockJobBusiness(ctx, tx, claim)
	if err != nil {
		return actions.Job{}, err
	}
	if owner.schedule == nil || owner.status != "running" || target == "" {
		return actions.Job{}, errors.New("schedule is not running")
	}
	p := owner.schedule
	job := actions.Job{JobID: idgen.New("schedact"), RunID: scheduledActionRunID(p.JobID, target), Type: actions.RunTypeCommand,
		ActorID: p.ActorID, Target: target, Command: p.Command, TimeoutSec: 30, RequestedAt: time.Now().UTC()}
	job.Params = map[string]string{"schedule_id": p.ScheduleID, "schedule_job_id": p.JobID,
		"scheduled_for": p.ScheduledFor.UTC().Format(time.RFC3339Nano), "schedule_action_job_id": job.JobID}
	params, err := marshalStringMap(job.Params)
	if err != nil {
		return actions.Job{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO action_runs(id,type,actor_id,target,command,params,dry_run,status,output,error,created_at,updated_at)
 VALUES($1,$2,$3,$4,$5,$6::jsonb,FALSE,'running','','',$7,$7)`, job.RunID, job.Type, job.ActorID, job.Target, job.Command, params, job.RequestedAt)
	if err != nil {
		return actions.Job{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return actions.Job{}, err
	}
	return job, nil
}

func (s *PostgresStore) FinishScheduledAction(parent context.Context, claim *jobqueue.Job, child actions.Job, result actions.Result) error {
	ctx, cancel := context.WithTimeout(parent, jobExecutionTimeout)
	defer cancel()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackJobExecution(tx)
	if err = lockJobClaim(ctx, tx, claim, false); err != nil {
		return err
	}
	owner, err := lockJobBusiness(ctx, tx, claim)
	if err != nil {
		return err
	}
	if owner.schedule == nil || (owner.status != "running" && owner.status != "superseded") {
		return errors.New("schedule is not running")
	}
	p := owner.schedule
	if child.RunID != scheduledActionRunID(p.JobID, child.Target) || child.ActorID != p.ActorID || child.Command != p.Command || child.Type != actions.RunTypeCommand || child.DryRun ||
		child.Params["schedule_id"] != p.ScheduleID || child.Params["schedule_job_id"] != p.JobID ||
		child.Params["scheduled_for"] != p.ScheduledFor.UTC().Format(time.RFC3339Nano) || child.Params["schedule_action_job_id"] != child.JobID ||
		result.JobID != child.JobID || result.RunID != child.RunID {
		return errors.New("scheduled child identity mismatch")
	}
	state, err := lockActionJob(ctx, tx, child)
	if err != nil {
		return err
	}
	if state != "running" {
		return errors.New("scheduled child is not running")
	}
	if err = writeActionJobResult(ctx, tx, result); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Public action creation always generates actrun IDs. A reserved deterministic
// child ID makes occurrence ownership independent of caller-controlled Params,
// and the primary key prevents dispatching the same target twice in an occurrence.
func scheduledActionRunID(jobID, target string) string {
	sum := sha256.Sum256([]byte(jobID + "\x00" + target))
	return "schedrun_" + hex.EncodeToString(sum[:])
}
