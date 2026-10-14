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
	ApprovedWorkload    *Workload
	Providers           map[string]Provider
	Verifier            Verifier
	Environment         Environment
	Evidence            EvidenceStore
	Activity            func(model.AgentActivity)
	CallActivity        func(model.AgentCallActivity)
}

func (controller *Controller) Run(ctx context.Context) error {
	if err := controller.validateExecution(); err != nil {
		return err
	}
	entries, err := controller.Evidence.Read(ctx)
	if err != nil {
		return err
	}
	if questions := unansweredQuestions(entries); len(questions) > 0 {
		return &QuestionsPending{Questions: questions}
	}
	workload, err := controller.frozenWorkload(ctx, entries)
	if err != nil {
		return err
	}
	state := projectTaskState(workload, entries)
	for ctx.Err() == nil {
		for _, task := range workload.Tasks {
			if state.accepted[task.ID] {
				continue
			}
			if err := controller.runTask(ctx, task, state); err != nil {
				return err
			}
		}
		if err := controller.Environment.Attest(ctx); err != nil {
			return err
		}
		checks, err := controller.executeChecks(ctx, controller.overallAssignment(workload), workload.Checks)
		if err != nil {
			return err
		}
		if err := validateCheckResults(workload.Checks, checks); err != nil {
			return err
		}
		assignment := controller.overallAssignment(workload)
		assignment.Checks = checks
		review, result, err := controller.review(ctx, assignment, state.tasks, state.round+1)
		if err != nil {
			return err
		}
		if len(review.Questions) > 0 {
			return controller.questions(ctx, "", review.Questions)
		}
		if !checksPass(checks) && len(review.Findings) == 0 {
			return controller.questions(ctx, "", []string{"Overall verification failed without a task-bound diagnosis. Clarify the failed checks or the frozen task scope before execution continues."})
		}
		if len(review.Findings) == 0 {
			_, err = controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "overall_accepted", CallID: result.CallID, Checks: checks})
			return err
		}
		for _, finding := range review.Findings {
			state.accepted[finding.TaskID] = false
			state.started[finding.TaskID] = false
			state.findings[finding.TaskID] = append(state.findings[finding.TaskID], finding)
			if _, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "task_reopened", TaskID: finding.TaskID, Findings: []Finding{finding}}); err != nil {
				return err
			}
		}
		for _, task := range workload.Tasks {
			for _, dependency := range task.Dependencies {
				if state.accepted[task.ID] && !state.accepted[dependency] {
					state.accepted[task.ID] = false
					if _, err := controller.Evidence.Append(ctx, Entry{Generation: controller.Generation, Phase: "task_reopened", TaskID: task.ID}); err != nil {
						return err
					}
				}
			}
		}
		state.round++
	}
	return ctx.Err()
}

func (controller *Controller) validate() error {
	if err := controller.validatePlanning(); err != nil {
		return err
	}
	if controller.Verifier == nil {
		return fmt.Errorf("agent controller lacks verification authority")
	}
	return nil
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

func (controller *Controller) validateExecution() error {
	if err := controller.validate(); err != nil {
		return err
	}
	if controller.ApprovedWorkload == nil {
		return fmt.Errorf("agent execution requires one explicitly approved workload")
	}
	for _, route := range []Route{controller.Configuration.Executor, controller.Configuration.Reviewer} {
		if controller.Providers[route.Provider] == nil {
			return fmt.Errorf("agent provider %q is not registered", route.Provider)
		}
	}
	return nil
}

func (controller *Controller) frozenWorkload(ctx context.Context, entries []Entry) (Workload, error) {
	workload := *controller.ApprovedWorkload
	if err := workload.Validate(); err != nil {
		return Workload{}, err
	}
	if err := bindWorkloadRequirements(&workload, controller.Requirements); err != nil {
		return Workload{}, err
	}
	return workload, nil
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
