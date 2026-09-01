package main

import (
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
		{ID: 1, StepID: 11, Key: "event", Value: "time=2026-02-15T00:00:00Z event=coding_phase_changed phase=assembling"},
		{ID: 2, StepID: 11, Key: "tool_stdout", Value: "running test: go test ./..."},
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
		{ID: 1, StepID: 11, Key: "event", Value: "time=2026-02-15T00:00:00Z event=coding_phase_changed phase=assembling"},
	}
	seen := map[int64]struct{}{}
	if printContextUpdates(contexts, seen, false, false, 1200) {
		t.Fatal("expected no context printing when progress and verbose are disabled")
	}
}

func TestPrintContextUpdatesSlimShowsCurrentCodingProgress(t *testing.T) {
	contexts := []model.StepContext{
		{
			ID:     1,
			StepID: 11,
			Key:    "event",
			Value:  "time=2026-07-28T00:00:00Z event=coding_stage_started attempt=1 generated_blocks=4",
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
		t.Fatal("expected current coding progress to show in slim progress mode")
	}
}

func TestSlimProgressShowsCodingMilestonesButNotHeartbeatNoise(t *testing.T) {
	for _, eventType := range []string{
		"coding_phase_changed",
		"coding_assembly_ready",
		"coding_stage_started",
		"coding_stage_passed",
		"coding_target_tree_validation_failed",
		"coding_compiler_repair_applied",
		"coding_portable_correction_dispatched",
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
		{event: stepEventPayload{EventType: "coding_phase_changed", Message: "phase=assembling detail=compiling_source"}, want: "Compiling deterministic source assembly"},
		{event: stepEventPayload{EventType: "coding_assembly_ready", Message: "adapter=go_cli files=6"}, want: "Deterministic assembly ready: 6 source units"},
		{event: stepEventPayload{EventType: "coding_target_tree_validation_failed", Message: "diagnostic=wrong file count"}, want: "wrong file count"},
		{event: stepEventPayload{EventType: "coding_worker_rejected", Message: "kind=semantic subject=application_surface model=qwen3.5:9b attempt=1/1 error=raw leaf is malformed"}, want: "Semantic station rejected application_surface (1/1): raw leaf is malformed"},
		{event: stepEventPayload{EventType: "coding_worker_failed", Message: "kind=fragment subject=feature.001 model=qwen3-coder:30b attempt=1/1 error=source node is malformed"}, want: "Source station failed for feature.001: source node is malformed"},
		{event: stepEventPayload{EventType: "coding_compiler_repair_applied", Message: "block=feature.001 mechanism=deterministic_primitive_nullish_narrowing"}, want: "Applied deterministic compiler repair to feature.001"},
		{event: stepEventPayload{EventType: "coding_portable_correction_dispatched", Message: "kind=fragment_generation work=abc123 iteration=2 model=qwen3.5:9b defect=92B"}, want: "Continuing source job abc123 in the same model context (iteration 2)"},
	}
	for _, test := range cases {
		if got := summarizeStepEvent(test.event); !strings.Contains(got, test.want) {
			t.Errorf("summary=%q want substring %q", got, test.want)
		}
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

func TestPhaseForStepActionUsesCurrentExecutableActions(t *testing.T) {
	if got := phaseForStepAction("v3_coding"); got != "coding" {
		t.Fatalf("phaseForStepAction(v3_coding)=%q want coding", got)
	}
	if got := phaseForStepAction("objective_resolve"); got != "objective" {
		t.Fatalf("phaseForStepAction(objective_resolve)=%q want objective", got)
	}
}
