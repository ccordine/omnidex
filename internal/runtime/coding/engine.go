package coding

import (
	"context"
	"fmt"
	"strings"
)

const (
	StatusPass    = "pass"
	StatusFail    = "fail"
	StatusBlocked = "blocked"
)

type Request struct {
	Goal      string
	Contexts  map[string]string
	Workspace string
}

type Input = Request

type Result struct {
	Summary string
	Events  []string
	Plan    CodingPlan
}

type EngineConfig struct {
	Interpreter         Interpreter
	Planner             Planner
	PlanValidator       PlanValidator
	Architect           Architect
	ArchitectValidator  ArchitectValidator
	Tester              Tester
	Coder               Coder
	ChangeValidator     ChangeValidator
	TaskValidator       TaskValidator
	EmptyScanner        EmptyScanner
	FinalSummarizer     FinalSummarizer
	MaxDispositionLoops int
}

type Interpreter interface {
	Interpret(ctx context.Context, req Request) (Request, error)
}

type Planner interface {
	Plan(ctx context.Context, req Request) (CodingPlan, error)
	Disposition(ctx context.Context, plan CodingPlan, report EmptyFileReport) (PlannerDisposition, error)
}

type Architect interface {
	Queue(ctx context.Context, task CodingPlannerTask) (ArchitectQueue, error)
}

type Tester = TestWriter
type ArchitectStepValidator = ArchitectValidator
type TaskValidator = ArchitectTaskValidator
type EmptyScanner = EmptyFileScanner

type TestWriter interface {
	WriteTest(ctx context.Context, step ChangeStep) error
}

type Coder interface {
	ApplyChange(ctx context.Context, step ChangeStep) error
}

type PlanValidator interface {
	ValidatePlan(ctx context.Context, plan CodingPlan) (ValidationResult, error)
}

type ArchitectValidator interface {
	ValidateArchitect(ctx context.Context, task CodingPlannerTask, queue ArchitectQueue) (ValidationResult, error)
}

type ChangeValidator interface {
	ValidateChange(ctx context.Context, step ChangeStep) (ValidationResult, error)
}

type ArchitectTaskValidator interface {
	ValidateArchitectTask(ctx context.Context, task CodingPlannerTask, queue ArchitectQueue) (ValidationResult, error)
}

type EmptyFileScanner interface {
	ScanEmptyFiles(ctx context.Context, workspace string) (EmptyFileReport, error)
}

type FinalSummarizer interface {
	Summarize(ctx context.Context, result Result) (string, error)
}

type Engine struct {
	Interpreter            Interpreter
	Planner                Planner
	PlanValidator          PlanValidator
	Architect              Architect
	ArchitectValidator     ArchitectValidator
	TestWriter             TestWriter
	Coder                  Coder
	ChangeValidator        ChangeValidator
	ArchitectTaskValidator ArchitectTaskValidator
	EmptyFileScanner       EmptyFileScanner
	FinalSummarizer        FinalSummarizer
	MaxDispositionLoops    int
}

func NewEngine(config EngineConfig) *Engine {
	return &Engine{
		Interpreter:            config.Interpreter,
		Planner:                config.Planner,
		PlanValidator:          config.PlanValidator,
		Architect:              config.Architect,
		ArchitectValidator:     config.ArchitectValidator,
		TestWriter:             config.Tester,
		Coder:                  config.Coder,
		ChangeValidator:        config.ChangeValidator,
		ArchitectTaskValidator: config.TaskValidator,
		EmptyFileScanner:       config.EmptyScanner,
		FinalSummarizer:        config.FinalSummarizer,
		MaxDispositionLoops:    config.MaxDispositionLoops,
	}
}

func NewDeterministicEngine() *Engine {
	validator := passValidator{}
	return &Engine{
		Interpreter:            identityInterpreter{},
		Planner:                deterministicPlanner{},
		PlanValidator:          validator,
		Architect:              deterministicArchitect{},
		ArchitectValidator:     validator,
		TestWriter:             noopTestWriter{},
		Coder:                  noopCoder{},
		ChangeValidator:        validator,
		ArchitectTaskValidator: validator,
		EmptyFileScanner:       NewEmptyFileScanner("."),
		FinalSummarizer:        deterministicSummarizer{},
		MaxDispositionLoops:    2,
	}
}

func (e *Engine) Run(ctx context.Context, req Request) (Result, error) {
	if e == nil {
		return Result{}, fmt.Errorf("coding engine is nil")
	}
	if err := e.ensureDefaults(); err != nil {
		return Result{}, err
	}

	interpreted, err := e.Interpreter.Interpret(ctx, req)
	if err != nil {
		return Result{}, err
	}
	events := []string{"interpreter"}

	plan, err := e.Planner.Plan(ctx, interpreted)
	if err != nil {
		return Result{}, err
	}
	events = append(events, "planner")
	if err := requirePass(ctx, e.PlanValidator.ValidatePlan, plan, "plan validator"); err != nil {
		return Result{}, err
	}
	events = append(events, "plan_validator")

	if err := e.runPlanTasks(ctx, plan, &events); err != nil {
		return Result{}, err
	}

	maxLoops := e.MaxDispositionLoops
	if maxLoops <= 0 {
		maxLoops = 1
	}
	var disposition PlannerDisposition
	for i := 0; i < maxLoops; i++ {
		report, err := e.EmptyFileScanner.ScanEmptyFiles(ctx, interpreted.Workspace)
		if err != nil {
			return Result{}, err
		}
		events = append(events, "empty_file_scanner")

		disposition, err = e.Planner.Disposition(ctx, plan, report)
		if err != nil {
			return Result{}, err
		}
		events = append(events, "planner_disposition")
		if disposition.Complete {
			break
		}
		if err := e.runDispositionActions(ctx, disposition, &events); err != nil {
			return Result{}, err
		}
	}
	if !disposition.Complete {
		return Result{}, fmt.Errorf("planner disposition did not complete")
	}

	result := Result{Events: events, Plan: plan}
	summary, err := e.FinalSummarizer.Summarize(ctx, result)
	if err != nil {
		return Result{}, err
	}
	result.Summary = summary
	result.Events = append(result.Events, "final_summary")
	return result, nil
}

func (e *Engine) runPlanTasks(ctx context.Context, plan CodingPlan, events *[]string) error {
	for _, task := range plan.Tasks {
		queue, err := e.Architect.Queue(ctx, task)
		if err != nil {
			return err
		}
		*events = append(*events, "architect")
		if err := requirePassArchitect(ctx, e.ArchitectValidator.ValidateArchitect, task, queue, "architect validator"); err != nil {
			return err
		}
		*events = append(*events, "architect_step_validator")
		for _, step := range queue.Changes() {
			if stepRequiresTest(step) {
				if err := e.TestWriter.WriteTest(ctx, step); err != nil {
					return err
				}
				*events = append(*events, "test_writer")
			}
			if err := e.Coder.ApplyChange(ctx, step); err != nil {
				return err
			}
			*events = append(*events, "coder")
			if err := requirePass(ctx, e.ChangeValidator.ValidateChange, step, "change validator"); err != nil {
				return err
			}
			*events = append(*events, "change_validator")
		}
		if err := requirePassTask(ctx, e.ArchitectTaskValidator.ValidateArchitectTask, task, queue, "architect task validator"); err != nil {
			return err
		}
		*events = append(*events, "architect_task_validator")
	}
	return nil
}

func (e *Engine) runDispositionActions(ctx context.Context, disposition PlannerDisposition, events *[]string) error {
	for _, action := range disposition.Actions {
		switch strings.ToLower(strings.TrimSpace(action.Action)) {
		case "ignore":
			continue
		case "fill", "delete":
			task := CodingPlannerTask{
				ID:              "planner_disposition:" + action.Path,
				Objective:       action.Action + " " + action.Path,
				SuccessCriteria: []string{action.Reason},
				Scope:           []string{action.Path},
			}
			if err := e.runPlanTasks(ctx, CodingPlan{Goal: task.Objective, Tasks: []CodingPlannerTask{task}}, events); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported planner disposition action %q for %s", action.Action, action.Path)
		}
	}
	return nil
}

func (e *Engine) ensureDefaults() error {
	defaults := NewDeterministicEngine()
	if e.Interpreter == nil {
		e.Interpreter = defaults.Interpreter
	}
	if e.Planner == nil {
		e.Planner = defaults.Planner
	}
	if e.PlanValidator == nil {
		e.PlanValidator = defaults.PlanValidator
	}
	if e.Architect == nil {
		e.Architect = defaults.Architect
	}
	if e.ArchitectValidator == nil {
		e.ArchitectValidator = defaults.ArchitectValidator
	}
	if e.TestWriter == nil {
		e.TestWriter = defaults.TestWriter
	}
	if e.Coder == nil {
		e.Coder = defaults.Coder
	}
	if e.ChangeValidator == nil {
		e.ChangeValidator = defaults.ChangeValidator
	}
	if e.ArchitectTaskValidator == nil {
		e.ArchitectTaskValidator = defaults.ArchitectTaskValidator
	}
	if e.EmptyFileScanner == nil {
		e.EmptyFileScanner = defaults.EmptyFileScanner
	}
	if e.FinalSummarizer == nil {
		e.FinalSummarizer = defaults.FinalSummarizer
	}
	return nil
}

func stepRequiresTest(step ChangeStep) bool {
	return step.RequiresTest || strings.EqualFold(strings.TrimSpace(step.Kind), "create_test")
}

func requirePass[T any](ctx context.Context, validate func(context.Context, T) (ValidationResult, error), value T, label string) error {
	result, err := validate(ctx, value)
	if err != nil {
		return err
	}
	return validateResult(result, label)
}

func requirePassTask(ctx context.Context, validate func(context.Context, CodingPlannerTask, ArchitectQueue) (ValidationResult, error), task CodingPlannerTask, queue ArchitectQueue, label string) error {
	result, err := validate(ctx, task, queue)
	if err != nil {
		return err
	}
	return validateResult(result, label)
}

func requirePassArchitect(ctx context.Context, validate func(context.Context, CodingPlannerTask, ArchitectQueue) (ValidationResult, error), task CodingPlannerTask, queue ArchitectQueue, label string) error {
	result, err := validate(ctx, task, queue)
	if err != nil {
		return err
	}
	return validateResult(result, label)
}

func validateResult(result ValidationResult, label string) error {
	switch result.Status {
	case StatusPass:
		return nil
	case StatusFail, StatusBlocked:
		return fmt.Errorf("%s %s: %s", label, result.Status, strings.Join(result.Violations, "; "))
	default:
		return fmt.Errorf("%s returned invalid status %q", label, result.Status)
	}
}
