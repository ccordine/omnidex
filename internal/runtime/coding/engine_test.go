package coding

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestEngineRunsPlannerArchitectChangeLoopInOrder(t *testing.T) {
	rec := newRecordingRuntime()
	rec.plan = CodingPlan{Goal: "fix bug", Tasks: []CodingPlannerTask{{
		ID:        "task_1",
		Objective: "fix bug",
	}}}
	rec.queues["task_1"] = ArchitectQueue{
		TaskID: "task_1",
		Writes: []ChangeStep{
			{ID: "change_1", Kind: "update_code", Intent: "first change", Validator: "unit", RequiresTest: true},
			{ID: "change_2", Kind: "update_code", Intent: "second change", Validator: "unit", RequiresTest: true},
		},
	}
	rec.dispositions = []PlannerDisposition{{Complete: true}}

	engine := engineFromRecorder(rec)
	if _, err := engine.Run(context.Background(), Request{Goal: "fix bug"}); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"interpreter",
		"planner",
		"plan_validator",
		"architect:task_1",
		"architect_validator:task_1",
		"tester:change_1",
		"coder:change_1",
		"change_validator:change_1",
		"tester:change_2",
		"coder:change_2",
		"change_validator:change_2",
		"task_validator:task_1",
		"empty_scanner",
		"planner_disposition",
		"final_summary",
	}
	if !reflect.DeepEqual(rec.calls, want) {
		t.Fatalf("calls=%v want %v", rec.calls, want)
	}
}

func TestPlannerOnlyCreatesTopLevelCodingTasks(t *testing.T) {
	planner := deterministicPlanner{}
	plan, err := planner.Plan(context.Background(), Request{Goal: "change this code correctly"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Goal == "" || len(plan.Tasks) != 1 {
		t.Fatalf("plan=%+v, want one top-level task", plan)
	}
	for _, task := range plan.Tasks {
		if task.ID == "" || task.Objective == "" {
			t.Fatalf("task must be concrete: %+v", task)
		}
	}
	taskType := reflect.TypeOf(CodingPlannerTask{})
	forbidden := []string{"Kind", "Path", "Validator", "Command", "Patch", "Writes", "Deletes", "Tools"}
	for _, field := range forbidden {
		if _, ok := taskType.FieldByName(field); ok {
			t.Fatalf("planner task must not directly create file edits or run tools; found field %q", field)
		}
	}
}

func TestArchitectCreatesConcreteQueues(t *testing.T) {
	architect := deterministicArchitect{}
	queue, err := architect.Queue(context.Background(), CodingPlannerTask{ID: "task_1", Objective: "update runtime"})
	if err != nil {
		t.Fatal(err)
	}
	if queue.TaskID != "task_1" {
		t.Fatalf("TaskID=%q want task_1", queue.TaskID)
	}
	if queue.Reads == nil || queue.Tests == nil || queue.Writes == nil || queue.Deletes == nil || queue.Validations == nil {
		t.Fatalf("architect must emit concrete read/test/write/delete/validation queues: %+v", queue)
	}
	if len(queue.Reads) == 0 || len(queue.Writes) == 0 || len(queue.Validations) == 0 {
		t.Fatalf("architect queue must include concrete read/write/validation queues: %+v", queue)
	}
	for _, step := range queue.Writes {
		if step.Kind == "" || step.Intent == "" || step.Validator == "" {
			t.Fatalf("write step must be concrete: %+v", step)
		}
	}
}

func TestCodingWorkflowDoesNotUseAssistantStages(t *testing.T) {
	engine := NewDeterministicEngine()
	result, err := engine.Run(context.Background(), Request{Goal: "change code correctly"})
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		"analysis",
		"response_draft",
		"external_research",
		"memory_review",
		"generic_verification",
		"v3_analysis",
		"v3_response_draft",
		"v3_external_research",
		"v3_memory_review",
		"v3_verification",
	}
	for _, stage := range forbidden {
		if containsCall(result.Events, stage) {
			t.Fatalf("coding workflow executed forbidden assistant stage %q in %v", stage, result.Events)
		}
	}
}

func TestChangeLoopOrder(t *testing.T) {
	rec := newRecordingRuntime()
	rec.plan = CodingPlan{Goal: "fix regression", Tasks: []CodingPlannerTask{{
		ID:        "task_1",
		Objective: "fix regression",
	}}}
	rec.queues["task_1"] = ArchitectQueue{
		TaskID: "task_1",
		Writes: []ChangeStep{
			{ID: "change_1", Kind: "update_code", Intent: "cover first", Validator: "unit", RequiresTest: true},
			{ID: "change_2", Kind: "update_code", Intent: "cover second", Validator: "unit", RequiresTest: true},
		},
	}
	rec.dispositions = []PlannerDisposition{{Complete: true}}

	if _, err := engineFromRecorder(rec).Run(context.Background(), Request{Goal: "fix regression"}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"tester:change_1",
		"coder:change_1",
		"change_validator:change_1",
		"tester:change_2",
		"coder:change_2",
		"change_validator:change_2",
	}
	if !containsInOrder(rec.calls, want) {
		t.Fatalf("change loop calls=%v want order %v", rec.calls, want)
	}
}

func TestPlannerDispositionLoopsBackThroughArchitectWhenWorkRemains(t *testing.T) {
	rec := newRecordingRuntime()
	rec.plan = CodingPlan{Goal: "clean empty file", Tasks: []CodingPlannerTask{{
		ID:        "task_1",
		Objective: "complete requested change",
	}}}
	rec.queues["task_1"] = ArchitectQueue{
		TaskID: "task_1",
		Writes: []ChangeStep{{
			ID:        "initial_change",
			Kind:      "update_code",
			Intent:    "initial implementation",
			Validator: "unit",
		}},
	}
	rec.queues["planner_disposition:empty.go"] = ArchitectQueue{
		TaskID: "planner_disposition:empty.go",
		Writes: []ChangeStep{{
			ID:        "fill_empty_file",
			Kind:      "update_code",
			Path:      "empty.go",
			Intent:    "fill empty file",
			Validator: "unit",
		}},
	}
	rec.emptyReport = EmptyFileReport{Files: []string{"empty.go"}}
	rec.dispositions = []PlannerDisposition{
		{
			Complete: false,
			Actions: []PlannerDispositionAction{{
				Path:   "empty.go",
				Action: "fill",
				Reason: "empty file must be filled",
			}},
		},
		{Complete: true},
	}

	engine := engineFromRecorder(rec)
	engine.MaxDispositionLoops = 3
	if _, err := engine.Run(context.Background(), Request{Goal: "clean empty file"}); err != nil {
		t.Fatal(err)
	}

	if !containsInOrder(rec.calls, []string{
		"planner_disposition",
		"architect:planner_disposition:empty.go",
		"architect_validator:planner_disposition:empty.go",
		"coder:fill_empty_file",
		"change_validator:fill_empty_file",
		"task_validator:planner_disposition:empty.go",
		"empty_scanner",
		"planner_disposition",
		"final_summary",
	}) {
		t.Fatalf("disposition work did not loop through architect/change validators: %v", rec.calls)
	}
	for _, forbidden := range []string{
		"v3_verification",
		"response_draft",
		"memory_review",
		"external_research",
		"evaluator_feedback",
	} {
		if containsCall(rec.calls, forbidden) {
			t.Fatalf("coding disposition loop called forbidden stage %q: %v", forbidden, rec.calls)
		}
	}
}

type recordingRuntime struct {
	calls        []string
	plan         CodingPlan
	queues       map[string]ArchitectQueue
	emptyReport  EmptyFileReport
	dispositions []PlannerDisposition
}

func newRecordingRuntime() *recordingRuntime {
	return &recordingRuntime{
		queues: map[string]ArchitectQueue{},
	}
}

func engineFromRecorder(rec *recordingRuntime) *Engine {
	return NewEngine(EngineConfig{
		Interpreter:         rec,
		Planner:             rec,
		PlanValidator:       rec,
		Architect:           rec,
		ArchitectValidator:  rec,
		Tester:              rec,
		Coder:               rec,
		ChangeValidator:     rec,
		TaskValidator:       rec,
		EmptyScanner:        rec,
		FinalSummarizer:     rec,
		MaxDispositionLoops: 2,
	})
}

func (r *recordingRuntime) Interpret(_ context.Context, in Request) (Request, error) {
	r.calls = append(r.calls, "interpreter")
	return in, nil
}

func (r *recordingRuntime) Plan(context.Context, Request) (CodingPlan, error) {
	r.calls = append(r.calls, "planner")
	return r.plan, nil
}

func (r *recordingRuntime) Disposition(context.Context, CodingPlan, EmptyFileReport) (PlannerDisposition, error) {
	r.calls = append(r.calls, "planner_disposition")
	if len(r.dispositions) == 0 {
		return PlannerDisposition{Complete: true}, nil
	}
	disposition := r.dispositions[0]
	r.dispositions = r.dispositions[1:]
	return disposition, nil
}

func (r *recordingRuntime) ValidatePlan(context.Context, CodingPlan) (ValidationResult, error) {
	r.calls = append(r.calls, "plan_validator")
	return ValidationResult{Status: StatusPass, Evidence: []string{"plan ok"}}, nil
}

func (r *recordingRuntime) Queue(_ context.Context, task CodingPlannerTask) (ArchitectQueue, error) {
	r.calls = append(r.calls, "architect:"+task.ID)
	queue, ok := r.queues[task.ID]
	if !ok {
		queue = ArchitectQueue{TaskID: task.ID}
	}
	return queue, nil
}

func (r *recordingRuntime) ValidateArchitect(_ context.Context, task CodingPlannerTask, queue ArchitectQueue) (ValidationResult, error) {
	if task.ID != queue.TaskID {
		return ValidationResult{Status: StatusFail, Violations: []string{"queue task_id does not match planner task"}}, nil
	}
	r.calls = append(r.calls, "architect_validator:"+queue.TaskID)
	return ValidationResult{Status: StatusPass, Evidence: []string{"queue ok"}}, nil
}

func (r *recordingRuntime) WriteTest(_ context.Context, step ChangeStep) error {
	r.calls = append(r.calls, "tester:"+step.ID)
	return nil
}

func (r *recordingRuntime) ApplyChange(_ context.Context, step ChangeStep) error {
	r.calls = append(r.calls, "coder:"+step.ID)
	return nil
}

func (r *recordingRuntime) ValidateChange(_ context.Context, step ChangeStep) (ValidationResult, error) {
	r.calls = append(r.calls, "change_validator:"+step.ID)
	return ValidationResult{Status: StatusPass, Evidence: []string{"change ok"}}, nil
}

func (r *recordingRuntime) ValidateArchitectTask(_ context.Context, task CodingPlannerTask, _ ArchitectQueue) (ValidationResult, error) {
	r.calls = append(r.calls, "task_validator:"+task.ID)
	return ValidationResult{Status: StatusPass, Evidence: []string{"task ok"}}, nil
}

func (r *recordingRuntime) ScanEmptyFiles(context.Context, string) (EmptyFileReport, error) {
	r.calls = append(r.calls, "empty_scanner")
	return r.emptyReport, nil
}

func (r *recordingRuntime) Summarize(context.Context, Result) (string, error) {
	r.calls = append(r.calls, "final_summary")
	return "done", nil
}

func containsInOrder(events, want []string) bool {
	offset := 0
	for _, event := range events {
		if offset < len(want) && event == want[offset] {
			offset++
		}
	}
	return offset == len(want)
}

func containsCall(calls []string, forbidden string) bool {
	for _, call := range calls {
		if strings.Contains(call, forbidden) {
			return true
		}
	}
	return false
}
