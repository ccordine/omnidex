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

func TestRoleForLLMScope(t *testing.T) {
	if got := roleForLLMScope("analyze"); got != "analysis_specialist" {
		t.Fatalf("roleForLLMScope(analyze)=%q, want %q", got, "analysis_specialist")
	}
	if got := roleForLLMScope("response_draft"); got != "response_specialist" {
		t.Fatalf("roleForLLMScope(response_draft)=%q, want %q", got, "response_specialist")
	}
	if got := roleForLLMScope("unknown_scope"); got != "" {
		t.Fatalf("roleForLLMScope(unknown_scope)=%q, want empty", got)
	}
}

func TestSummarizePreparedModelContext(t *testing.T) {
	kind, summary := summarizePreparedModelContext("scope=analyze\nbase_model=qwen3:14b\ncontext_model=ctx-qwen3-1234", 240)
	if kind != "Model" {
		t.Fatalf("kind=%q want Model", kind)
	}
	if !strings.Contains(summary, "role=analysis_specialist") {
		t.Fatalf("expected role in summary, got: %q", summary)
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
