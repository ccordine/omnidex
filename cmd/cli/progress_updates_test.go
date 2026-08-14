package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPrintStepStatusUpdatesOnlyOnChange(t *testing.T) {
	steps := []model.Step{
		{ID: 1, Action: "plan", Status: model.StepStatusRunning},
	}
	state := map[int64]string{}

	if !printStepStatusUpdates(steps, state) {
		t.Fatal("expected first status update to print")
	}
	if printStepStatusUpdates(steps, state) {
		t.Fatal("expected unchanged status to be suppressed")
	}
}

func TestFormatWorkloadQueueStatusLineShowsActiveAndCounts(t *testing.T) {
	steps := []model.Step{
		{ID: 1, Action: "plan", Status: model.StepStatusCompleted},
		{ID: 2, Action: "assist", Status: model.StepStatusRunning},
		{ID: 3, Action: "verify", Status: model.StepStatusPending},
	}

	line := formatWorkloadQueueStatusLine(steps, nil)

	for _, want := range []string{"active=#2 assist", "completed=1", "incomplete=2"} {
		if !strings.Contains(line, want) {
			t.Fatalf("workload line missing %q: %q", want, line)
		}
	}
}

func TestFormatStepStatusLineMarksActiveCompletedAndPending(t *testing.T) {
	active := formatStepStatusLine(model.Step{ID: 2, Action: "assist", Status: model.StepStatusRunning}, nil)
	done := formatStepStatusLine(model.Step{ID: 1, Action: "plan", Status: model.StepStatusCompleted}, nil)
	todo := formatStepStatusLine(model.Step{ID: 3, Action: "verify", Status: model.StepStatusPending}, nil)

	for _, pair := range []struct {
		line string
		want string
	}{
		{active, ">> ACTIVE"},
		{done, "OK DONE"},
		{todo, ".. TODO"},
	} {
		if !strings.Contains(pair.line, pair.want) {
			t.Fatalf("line %q missing %q", pair.line, pair.want)
		}
	}
}

func TestPrintContextUpdatesProgressMode(t *testing.T) {
	contexts := []model.StepContext{
		{ID: 1, StepID: 11, Key: "event", Value: "time=2026-02-15T00:00:00Z event=plan_begin"},
		{ID: 2, StepID: 11, Key: "tool_stdout", Value: "running test: go test ./..."},
		{ID: 3, StepID: 11, Key: "environment", Value: "env_cwd=/tmp"},
	}
	seen := map[int64]struct{}{}

	if !printContextUpdates(contexts, seen, true, false, 200) {
		t.Fatal("expected progress contexts to print")
	}
	if printContextUpdates(contexts, seen, true, false, 200) {
		t.Fatal("expected seen contexts to be suppressed")
	}
}

func TestPrintContextUpdatesDisabled(t *testing.T) {
	contexts := []model.StepContext{
		{ID: 1, StepID: 11, Key: "event", Value: "time=2026-02-15T00:00:00Z event=plan_begin"},
	}
	seen := map[int64]struct{}{}
	if printContextUpdates(contexts, seen, false, false, 1200) {
		t.Fatal("expected no context printing when progress and verbose are disabled")
	}
}

func TestPrintContextUpdatesSlimHidesLLMPromptTrace(t *testing.T) {
	contexts := []model.StepContext{
		{ID: 1, StepID: 11, Key: "llm_prompt", Value: "very long prompt"},
	}
	seen := map[int64]struct{}{}
	if printContextUpdates(contexts, seen, true, false, 1200) {
		t.Fatal("expected llm prompt trace to be hidden in slim progress mode")
	}
}

func TestPrintContextUpdatesSlimShowsModelStreamProgress(t *testing.T) {
	contexts := []model.StepContext{
		{
			ID:     1,
			StepID: 11,
			Key:    "event",
			Value:  "time=2026-07-28T00:00:00Z event=llm_stream_progress scope=v3_source_turn_3 output_bytes=2048 elapsed=12s",
		},
		{
			ID:     2,
			StepID: 11,
			Key:    "event",
			Value:  "time=2026-07-28T00:00:01Z event=operation_heartbeat scope=v3_source_turn_3 elapsed=13s",
		},
	}
	seen := map[int64]struct{}{}
	if !printContextUpdates(contexts, seen, true, false, 1200) {
		t.Fatal("expected model stream progress to show in slim progress mode")
	}
}

func TestSlimProgressShowsCodingMilestonesButNotHeartbeatNoise(t *testing.T) {
	for _, eventType := range []string{
		"coding_phase_changed",
		"coding_assembly_ready",
		"coding_file_started",
		"coding_file_written",
		"coding_file_deleted",
		"coding_file_unchanged",
		"coding_verification_failed",
		"coding_static_validation_failed",
		"coding_repair_selected",
		"coding_fragment_correction_started",
		"coding_worker_rejected",
		"coding_worker_failed",
	} {
		if !showStepEventInSlimProgress(eventType) {
			t.Fatalf("coding milestone %q is hidden in slim progress", eventType)
		}
	}
	for _, eventType := range []string{"operation_heartbeat", "step_authority_poll", "coding_verification_command_passed", "coding_worker_started", "coding_worker_completed"} {
		if showStepEventInSlimProgress(eventType) {
			t.Fatalf("redundant event %q is visible in slim progress", eventType)
		}
	}
	if showStepEventInSlimProgress("coding_manifest_ready") {
		t.Fatal("removed manifest event is still visible")
	}
	if showStepEventInSlimProgress("coding_model_response_rejected") {
		t.Fatal("removed untyped model rejection event is still visible")
	}
}

func TestSummarizeStepEventExplainsCodingState(t *testing.T) {
	cases := []struct {
		event stepEventPayload
		want  string
	}{
		{event: stepEventPayload{EventType: "coding_phase_changed", Message: "phase=verifying detail=running_server_checks"}, want: "Verifying accepted workspace"},
		{event: stepEventPayload{EventType: "coding_assembly_ready", Message: "adapter=go_cli files=6"}, want: "Deterministic assembly ready: 6 source units"},
		{event: stepEventPayload{EventType: "coding_static_validation_failed", Message: "diagnostic=go.mod declares Go 1.21; requirement R1 requires Go 1.22"}, want: "go.mod declares Go 1.21"},
		{event: stepEventPayload{EventType: "coding_worker_rejected", Message: "kind=repair subject=go_test model=qwen2.5-coder:3b attempt=1/3 error=repair must target main.go"}, want: "Repair station rejected go_test (1/3): repair must target main.go"},
		{event: stepEventPayload{EventType: "coding_worker_failed", Message: "kind=file subject=main.go model=qwen3-coder:30b attempt=3/3 error=typed response is malformed"}, want: "File station failed for main.go: typed response is malformed"},
		{event: stepEventPayload{EventType: "coding_file_written", Message: "stage=generate path=store.go bytes=812"}, want: "Accepted store.go (812 bytes)"},
		{event: stepEventPayload{EventType: "coding_repair_selected", Message: "repair=1 path=main_test.go command=go_test_./..."}, want: "Selected main_test.go for diagnostic repair 1"},
		{event: stepEventPayload{EventType: "coding_fragment_correction_started", Message: "block=feature.001 exact_failure=TypeError: AudioContext is not defined"}, want: "Correcting feature.001: TypeError: AudioContext is not defined"},
	}
	for _, test := range cases {
		if got := summarizeStepEvent(test.event); !strings.Contains(got, test.want) {
			t.Errorf("summary=%q want substring %q", got, test.want)
		}
	}
}

func TestPrintContextUpdatesSlimShowsLLMResponseTrace(t *testing.T) {
	contexts := []model.StepContext{
		{
			ID:     1,
			StepID: 11,
			Key:    "llm_response",
			Value:  "scope=analyze\nmodel=qwen3:14b\nresponse_chars=24\n- concise analysis output",
		},
	}
	seen := map[int64]struct{}{}
	if !printContextUpdates(contexts, seen, true, false, 1200) {
		t.Fatal("expected llm response trace to show in slim progress mode")
	}
}

func TestPrintContextUpdatesSlimShowsLLMModelPrepare(t *testing.T) {
	contexts := []model.StepContext{
		{
			ID:     1,
			StepID: 11,
			Key:    "llm_model_prepare",
			Value:  "scope=analyze\nbase_model=qwen3:14b\ncontext_model=ctx-qwen3-1234\nmodelfile_path=/tmp/model.Modelfile",
		},
	}
	seen := map[int64]struct{}{}
	if !printContextUpdates(contexts, seen, true, false, 1200) {
		t.Fatal("expected llm model-prepare context to show in slim progress mode")
	}
}

func TestPrintContextUpdatesVerboseShowsLLMTrace(t *testing.T) {
	contexts := []model.StepContext{
		{ID: 1, StepID: 11, Key: "llm_prompt", Value: "very long prompt"},
	}
	seen := map[int64]struct{}{}
	if !printContextUpdates(contexts, seen, true, true, 1200) {
		t.Fatal("expected llm trace to show in verbose mode")
	}
}

func TestLLMTraceBody(t *testing.T) {
	raw := "scope=response_draft\nmodel=qwen3:14b\nresponse_chars=20\nline one\nline two"
	got := llmTraceBody(raw)
	want := "line one\nline two"
	if got != want {
		t.Fatalf("llmTraceBody()=%q, want %q", got, want)
	}
}

func TestSummarizePreparedModelContext(t *testing.T) {
	kind, summary := summarizePreparedModelContext("scope=analyze\nbase_model=qwen3:14b\ncontext_model=ctx-qwen3-1234", 240)
	if kind != "Model" {
		t.Fatalf("kind=%q want Model", kind)
	}
	if !strings.Contains(summary, "context_model=ctx-qwen3-1234") {
		t.Fatalf("expected context model in summary, got: %q", summary)
	}
}

func TestCompactProgressValue(t *testing.T) {
	value := compactProgressValue("line one\nline two", 8)
	if strings.Contains(value, "\n") {
		t.Fatalf("expected flattened value, got %q", value)
	}
	if !strings.Contains(value, "...[truncated]") {
		t.Fatalf("expected truncation marker, got %q", value)
	}
}

func TestWebSearchDomainsFromContext(t *testing.T) {
	context := strings.Join([]string{
		"Source: yahoo",
		"URL: https://us.search.yahoo.com/search?p=vlc+status",
		"Source: google",
		"URL: https://www.google.com/search?q=vlc+status",
		"Source: reddit",
		"URL: https://www.google.com/search?q=reddit+vlc+status",
	}, "\n")

	got := webSearchDomainsFromContext(context)
	want := []string{"us.search.yahoo.com", "www.google.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("webSearchDomainsFromContext()=%v, want %v", got, want)
	}
}

func TestSummarizeWebSearchDomains(t *testing.T) {
	domains := []string{"us.search.yahoo.com", "www.google.com"}
	got := summarizeWebSearchDomains(domains, 120)
	if !strings.Contains(got, "us.search.yahoo.com") || !strings.Contains(got, "www.google.com") {
		t.Fatalf("expected both domains in summary, got %q", got)
	}
}

func TestPhaseForStepActionPlanningResearchActions(t *testing.T) {
	for _, action := range []string{"tooling", "workspace_scan", "tag", "retrieve", "plan"} {
		if got := phaseForStepAction(action); got != "planning" {
			t.Fatalf("phaseForStepAction(%q)=%q want planning", action, got)
		}
	}
	if got := phaseForStepAction("verify"); got != "review" {
		t.Fatalf("phaseForStepAction(verify)=%q want review", got)
	}
	if got := phaseForStepAction("assist"); got != "execution" {
		t.Fatalf("phaseForStepAction(assist)=%q want execution", got)
	}
}
