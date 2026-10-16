package codingagent

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

type Controller struct {
	JobID, Generation   int64
	Root                string
	Configuration       Configuration
	Requirements        []Requirement
	Objective           *Objective
	ArtifactBindings    []ArtifactBinding
	PlanningEnvironment *PlanningEnvironment
	ConstructionFacts   []ConstructionFact
	Providers           map[string]Provider
	Verifier            Verifier
	Environment         Environment
	Evidence            EvidenceStore
	Activity            func(model.AgentActivity)
	CallActivity        func(model.AgentCallActivity)
}

// RunTask executes one database-selected ledger task and then opens a fresh,
// read-only review for that same task. It does not plan, load sibling tasks, or
// perform aggregate review.
func (controller *Controller) RunTask(ctx context.Context, task Task) error {
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
	return controller.runTask(ctx, task, state)
}

func (controller *Controller) validatePlanning() error {
	if controller == nil || controller.JobID < 1 || controller.Generation < 1 || controller.Root == "" || controller.Environment == nil || controller.Evidence == nil || len(controller.Requirements) == 0 {
		return fmt.Errorf("agent controller lacks task, environment or evidence authority")
	}
	if err := controller.Configuration.Validate(); err != nil {
		return err
	}
	if controller.Configuration.Mode != Agents {
		return fmt.Errorf("agent controller requires explicit Agents mode")
	}
	if route := controller.Configuration.Planner; controller.Providers[route.Provider] == nil {
		return fmt.Errorf("agent provider %q is not registered", route.Provider)
	}
	return nil
}

func (controller *Controller) validateTaskExecution() error {
	if controller == nil || controller.JobID < 1 || controller.Generation < 1 || controller.Root == "" ||
		controller.Environment == nil || controller.Evidence == nil || controller.Verifier == nil {
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
	return nil
}

func checksPass(checks []CheckResult) bool {
	if len(checks) == 0 {
		return false
	}
	for _, check := range checks {
		if check.ExitCode != 0 {
			return false
		}
	}
	return true
}
