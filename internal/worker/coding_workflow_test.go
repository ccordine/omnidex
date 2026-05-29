package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	runtimecoding "github.com/gryph/omnidex/internal/runtime/coding"
)

func TestCodingWorkflowProcessStepCannotUseAssistantRuntime(t *testing.T) {
	svc := newServiceWithPanickingAssistantRuntime(t)
	claim := &model.ClaimedStep{
		Job: model.Job{
			ID:          10,
			Instruction: "change this code correctly",
			Pipeline:    model.PipelineCoding,
		},
		Step: model.Step{
			ID:     20,
			Action: "coding_workflow",
		},
	}

	if err := svc.processStep(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
}

func TestCodingWorkflowCannotUseAssistantRuntime(t *testing.T) {
	svc := newServiceWithPanickingAssistantRuntime(t)
	claim := &model.ClaimedStep{
		Job: model.Job{
			ID:          10,
			Instruction: "change this code correctly",
			Pipeline:    model.PipelineCoding,
		},
		Step: model.Step{
			ID:     20,
			Action: "coding_workflow",
		},
	}

	if err := svc.runCodingWorkflowStep(context.Background(), claim, nil); err != nil {
		t.Fatal(err)
	}
}

func newServiceWithPanickingAssistantRuntime(t *testing.T) Service {
	t.Helper()
	panicIfCalled := func(label string) func(context.Context, *model.ClaimedStep, map[string]string, string) error {
		return func(context.Context, *model.ClaimedStep, map[string]string, string) error {
			panic("coding workflow must not call " + label)
		}
	}
	return Service{
		codingEngine: &recordingCodingEngine{
			result: runtimecoding.Result{Summary: "coding complete"},
		},
		completeStep: func(context.Context, int64, string, string, string) error {
			return nil
		},
		nativeV3Runner:     panicIfCalled("native v3 runtime"),
		agentRuntimeRunner: panicIfCalled("assistant runtime"),
	}
}

func TestCodingWorkflowUsesCodingEngineOnly(t *testing.T) {
	engine := &recordingCodingEngine{
		result: runtimecoding.Result{Summary: "coding complete"},
	}
	var completed struct {
		stepID       int64
		output       string
		contextKey   string
		contextValue string
	}
	svc := Service{
		codingEngine: engine,
		completeStep: func(_ context.Context, stepID int64, output, contextKey, contextValue string) error {
			completed.stepID = stepID
			completed.output = output
			completed.contextKey = contextKey
			completed.contextValue = contextValue
			return nil
		},
		nativeV3Runner: func(context.Context, *model.ClaimedStep, map[string]string, string) error {
			t.Fatalf("coding workflow must not call native v3 runtime")
			return nil
		},
		agentRuntimeRunner: func(context.Context, *model.ClaimedStep, map[string]string, string) error {
			t.Fatalf("coding workflow must not call assistant runtime")
			return nil
		},
	}
	claim := &model.ClaimedStep{
		Job: model.Job{
			ID:          10,
			Instruction: "change this code correctly",
			Pipeline:    model.PipelineCoding,
			Metadata:    json.RawMessage(`{"client_cwd":"/tmp/work"}`),
		},
		Step: model.Step{
			ID:     20,
			Action: "coding_workflow",
		},
	}

	if err := svc.processStep(context.Background(), claim); err != nil {
		t.Fatal(err)
	}
	if engine.calls != 1 {
		t.Fatalf("coding engine calls=%d want 1", engine.calls)
	}
	if engine.input.Goal != claim.Job.Instruction || engine.input.Workspace != "/tmp/work" {
		t.Fatalf("coding engine input=%+v", engine.input)
	}
	if completed.stepID != claim.Step.ID || completed.output != "coding complete" || completed.contextKey != "coding_workflow" {
		t.Fatalf("completed step=%+v", completed)
	}
	if !strings.Contains(completed.contextValue, "coding complete") {
		t.Fatalf("context value missing coding result: %q", completed.contextValue)
	}
}

func TestCodingJobRejectsAssistantRuntimeActions(t *testing.T) {
	svc := Service{}
	for _, action := range []string{
		"tooling",
		"workspace_scan",
		"tag",
		"retrieve",
		"plan",
		"planning",
		"web_search",
		"analyze",
		"assist",
		"roleplay",
		"narrate",
		"verify",
		"v3_intent_parse",
		"v3_planning",
		"v3_analysis",
		"v3_verification",
		"v3_response_draft",
		"v3_memory_review",
		"v3_external_research",
	} {
		claim := &model.ClaimedStep{
			Job:  model.Job{Pipeline: model.PipelineCoding},
			Step: model.Step{Action: action},
		}
		err := svc.processStep(context.Background(), claim)
		if err == nil || !strings.Contains(err.Error(), "coding pipeline cannot run non-coding action") {
			t.Fatalf("processStep action=%q err=%v, want coding rejection", action, err)
		}
	}
}

func TestCodingWorkspaceForJobPrefersClientCWD(t *testing.T) {
	job := model.Job{
		Pipeline: model.PipelineCoding,
		Metadata: json.RawMessage(`{
			"client_cwd":"/tmp/from-client",
			"workspace":"/tmp/from-workspace"
		}`),
	}
	if got := codingWorkspaceForJob(job); got != "/tmp/from-client" {
		t.Fatalf("codingWorkspaceForJob()=%q want client cwd", got)
	}
}

type recordingCodingEngine struct {
	calls  int
	input  runtimecoding.Request
	result runtimecoding.Result
}

func (e *recordingCodingEngine) Run(_ context.Context, in runtimecoding.Request) (runtimecoding.Result, error) {
	e.calls++
	e.input = in
	return e.result, nil
}
