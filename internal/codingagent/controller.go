package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

type Controller struct {
	JobID, Generation int64
	Root              string
	Configuration     Configuration
	Providers         map[string]Provider
	Environment       Environment
	Evidence          EvidenceStore
	Activity          func(model.AgentActivity)
	CallActivity      func(model.AgentCallActivity)
	NativeInput       func(context.Context, NativeInputRequest, int64, string) (json.RawMessage, error)
	CompletedTaskIDs  []string
}

// resumeTask executes and independently reviews one database-selected item.
// The outer loop selects the next item and calls ReviewGoal after the last one.
func (controller *Controller) resumeTask(ctx context.Context, task Task) error {
	if err := controller.validateTaskExecution(); err != nil {
		return err
	}
	if err := task.Validate(); err != nil {
		return err
	}
	entries, err := controller.Evidence.Read(ctx)
	if err != nil {
		return err
	}
	state := projectTaskState(task, entries)
	if state.accepted[task.ID] {
		return nil
	}
	if state.incomplete != nil && state.inputRequired == nil && state.pendingResponse == nil && state.pendingReview == nil && !recoverableSession(state.session) && !recoverableSession(state.reviewSession) {
		return state.incomplete
	}
	if state.pendingResponse != nil {
		kind, err := controller.handleExecutionResponse(ctx, task, *state.pendingResponse, state.round)
		if err != nil {
			var incomplete *IncompleteTaskError
			if !state.resumeIncomplete || !errors.As(err, &incomplete) {
				return err
			}
			// Reinterpretation confirmed actual remaining work. Only now consume the
			// explicit owner resume as native continuation authority.
			state.session = &ExecutionSession{ThreadID: incomplete.ThreadID, TurnID: incomplete.TurnID, State: "interrupted"}
			state.pendingResponse = nil
		} else {
			if err := state.recordExecutionResult(task.ID, *state.pendingResponse, state.round, kind); err != nil {
				return err
			}
		}
	}
	return controller.runTask(ctx, task, state)
}

func (controller *Controller) validatePlanning() error {
	if controller == nil || controller.JobID < 1 || controller.Generation < 1 || controller.Root == "" || controller.Environment == nil || controller.Evidence == nil {
		return fmt.Errorf("agent controller lacks task, environment or evidence authority")
	}
	if err := controller.Configuration.Validate(); err != nil {
		return err
	}
	if controller.Configuration.Mode != Agents {
		return fmt.Errorf("agent controller requires explicit Agents mode")
	}
	for _, route := range []Route{controller.Configuration.Planner, controller.Configuration.LedgerParser} {
		if err := validateRoute(route); err != nil {
			return err
		}
		if controller.Providers[route.Provider] == nil {
			return fmt.Errorf("agent provider %q is not registered", route.Provider)
		}
	}
	return nil
}

func (controller *Controller) validateTaskExecution() error {
	if controller == nil || controller.JobID < 1 || controller.Generation < 1 || controller.Root == "" ||
		controller.Environment == nil || controller.Evidence == nil {
		return fmt.Errorf("agent controller lacks task, environment, evidence, or verification authority")
	}
	if err := controller.Configuration.Validate(); err != nil {
		return err
	}
	if controller.Configuration.Mode != Agents {
		return fmt.Errorf("agent controller requires explicit Agents mode")
	}
	for _, route := range []Route{controller.Configuration.Executor, controller.Configuration.Reviewer} {
		if controller.Providers[route.Provider] == nil {
			return fmt.Errorf("agent provider %q is not registered", route.Provider)
		}
	}
	if err := validateRoute(controller.Configuration.LedgerParser); err != nil {
		return err
	}
	if controller.Providers[controller.Configuration.LedgerParser.Provider] == nil {
		return fmt.Errorf("execution response parser provider is not registered")
	}
	return nil
}

func recoverableSession(session *ExecutionSession) bool {
	return session != nil && session.ThreadID != "" && session.State != "idle"
}
