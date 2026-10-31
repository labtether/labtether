package persistence

import (
	"context"
	"testing"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/jobqueue"
	"github.com/labtether/labtether/internal/terminal"
)

// The worker stores agent results through FinishJobExecution. A command that
// exited 2 must read back as 2 for terminal commands and action run steps,
// and an unknown exit code must read back as unknown, never as 0.
func TestPostgresStoresCommandExitCodes(t *testing.T) {
	s := newTestPostgresStore(t)
	for _, exitCode := range []*int{new(2), nil} {
		session, err := s.CreateSession(terminal.CreateSessionRequest{ActorID: "owner", Target: "fixture-target", Mode: "structured"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.DeleteTerminalSession(session.ID) })
		cmd, err := s.AddCommand(session.ID, terminal.CreateCommandRequest{ActorID: "owner", Command: "ls missing"}, session.Target, session.Mode)
		if err != nil {
			t.Fatal(err)
		}
		p := terminal.CommandJob{JobID: idgen.New("exit-job"), SessionID: session.ID, CommandID: cmd.ID, ActorID: cmd.ActorID, Target: cmd.Target, Command: cmd.Body, Mode: cmd.Mode}
		job := seedExecutionClaim(t, s, jobqueue.KindTerminalCommand, p.JobID, p)
		finishExitCodeClaim(t, s, job, JobExecutionResult{Terminal: &terminal.CommandResult{
			JobID: p.JobID, SessionID: p.SessionID, CommandID: p.CommandID, Status: "failed", Output: "missing", ExitCode: exitCode,
		}})
		stored, _, err := s.GetCommand(cmd.ID)
		if err != nil {
			t.Fatal(err)
		}
		requireStoredExitCode(t, "terminal command", stored.ExitCode, exitCode)

		actionJob, action := actionExecutionClaim(t, s)
		finishExitCodeClaim(t, s, actionJob, JobExecutionResult{Action: &actions.Result{
			JobID: action.JobID, RunID: action.RunID, Status: actions.StatusFailed,
			Steps: []actions.StepResult{{Name: "execute_command", Status: actions.StatusFailed, ExitCode: exitCode}},
		}})
		run, _, err := s.GetActionRun(action.RunID)
		if err != nil || len(run.Steps) != 1 {
			t.Fatalf("action run steps = %+v, err %v", run.Steps, err)
		}
		requireStoredExitCode(t, "action step", run.Steps[0].ExitCode, exitCode)
	}
}

func finishExitCodeClaim(t *testing.T, s *PostgresStore, job *jobqueue.Job, result JobExecutionResult) {
	t.Helper()
	if ok, err := s.BeginJobExecution(context.Background(), job); err != nil || !ok {
		t.Fatalf("begin = %v/%v", ok, err)
	}
	if err := s.FinishJobExecution(context.Background(), job, result); err != nil {
		t.Fatal(err)
	}
}

func requireStoredExitCode(t *testing.T, what string, got, want *int) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("%s exit code = %d, want unknown", what, *got)
		}
		return
	}
	if got == nil || *got != *want {
		t.Fatalf("%s exit code = %v, want %d", what, got, *want)
	}
}
