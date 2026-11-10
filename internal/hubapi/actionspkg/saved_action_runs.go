package actionspkg

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/actions"
	"github.com/labtether/labtether/internal/apiv2"
	"github.com/labtether/labtether/internal/hubapi/shared"
	"github.com/labtether/labtether/internal/savedactions"
)

// savedStepOutcome is what one saved-action step did, as the run history
// stores it.
type savedStepOutcome struct {
	status   string
	output   string
	err      string
	exitCode *int
}

// deniedStepOutcome turns a step the hub refused (policy, maintenance window,
// time limit) into a failed run whose error says why.
func deniedStepOutcome(entry map[string]any) savedStepOutcome {
	message, _ := entry["message"].(string)
	if strings.TrimSpace(message) == "" {
		message, _ = entry["error"].(string)
	}
	return savedStepOutcome{status: actions.StatusFailed, err: message}
}

// execStepOutcome turns a dispatched step's result into a run outcome.
func execStepOutcome(result ExecResult) savedStepOutcome {
	if result.Error != "" {
		message := strings.TrimSpace(result.Message)
		if message == "" {
			message = result.Error
		}
		return savedStepOutcome{status: actions.StatusFailed, err: message}
	}
	status := actions.StatusSucceeded
	if result.Status != actions.StatusSucceeded {
		status = actions.StatusFailed
	}
	return savedStepOutcome{status: status, output: result.Stdout, exitCode: result.ExitCode}
}

// recordSavedActionStep stores a finished saved-action step as an action run,
// so it shows in Recent runs like a scheduled run. The run is written once
// the step has ended, so a hub restart never leaves it queued for good.
// Recording is best effort: the person who pressed Run still gets the result.
func (d *Deps) recordSavedActionStep(r *http.Request, action savedactions.SavedAction, index int, step savedactions.ActionStep, outcome savedStepOutcome) {
	if d.ActionStore == nil {
		return
	}
	run, err := d.ActionStore.CreateActionRun(actions.ExecuteRequest{
		ActorID: apiv2.PrincipalActorID(r.Context()),
		Type:    actions.RunTypeCommand,
		Target:  step.Target,
		Command: step.Command,
		Params: map[string]string{
			shared.SavedActionRunParam: action.ID,
			"saved_action_step":        strconv.Itoa(index + 1),
		},
	})
	if err != nil {
		log.Printf("saved action %s: failed to record step %d run: %v", action.ID, index+1, err)
		return
	}
	err = d.ActionStore.ApplyActionResult(actions.Result{
		RunID:  run.ID,
		Status: outcome.status,
		Output: outcome.output,
		Error:  outcome.err,
		Steps: []actions.StepResult{{
			Name:     step.Name,
			Status:   outcome.status,
			Output:   outcome.output,
			Error:    outcome.err,
			ExitCode: outcome.exitCode,
		}},
		CompletedAt: time.Now().UTC(),
	})
	if err != nil {
		log.Printf("saved action %s: failed to record step %d result: %v", action.ID, index+1, err)
	}
}
