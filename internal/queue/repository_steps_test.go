package queue

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
)

func TestBuildV3AuthorityRevisionMetadataPreservesOrderedUserAuthority(t *testing.T) {
	job := model.Job{
		ID:          41,
		Instruction: "Repair agent routing",
		Metadata: json.RawMessage(`{
			"runtime":"v3",
			"client_cwd":"/workspace/current",
			"telemetry_run_id":"old-run",
			"v3_authority_revision":2,
			"v3_root_job_id":17,
			"v3_authority_directives":["Keep the server authoritative"]
		}`),
	}
	raw, revision, rootID, err := buildV3AuthorityRevisionMetadata(job, "Validate every mutation")
	if err != nil {
		t.Fatal(err)
	}
	if revision != 3 || rootID != 17 {
		t.Fatalf("revision=%d root=%d", revision, rootID)
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["v3_parent_job_id"] != float64(41) || metadata["v3_authority_revision"] != float64(3) {
		t.Fatalf("revision linkage=%v", metadata)
	}
	directives, ok := metadata["v3_authority_directives"].([]any)
	if !ok || !reflect.DeepEqual(directives, []any{"Keep the server authoritative", "Validate every mutation"}) {
		t.Fatalf("authority directives=%#v", metadata["v3_authority_directives"])
	}
	if _, exists := metadata["telemetry_run_id"]; exists {
		t.Fatalf("old telemetry identity leaked into successor metadata: %v", metadata)
	}
	if metadata["client_cwd"] != "/workspace/current" {
		t.Fatalf("authoritative workspace scope was not preserved: %v", metadata)
	}
}

func TestBuildV3AuthorityRevisionMetadataRejectsMalformedAuthority(t *testing.T) {
	tests := []model.Job{
		{ID: 1, Metadata: json.RawMessage(`{"runtime":"v3","v3_authority_revision":"two"}`)},
		{ID: 1, Metadata: json.RawMessage(`{"runtime":"v3","v3_authority_directives":"build music"}`)},
		{ID: 1, Metadata: json.RawMessage(`{"runtime":"v3","v3_root_job_id":0}`)},
	}
	for _, job := range tests {
		if _, _, _, err := buildV3AuthorityRevisionMetadata(job, "repair routing"); err == nil {
			t.Fatalf("malformed authority metadata must fail: %s", job.Metadata)
		}
	}
	if _, _, _, err := buildV3AuthorityRevisionMetadata(model.Job{ID: 1, Metadata: json.RawMessage(`{"runtime":"v3"}`)}, "  "); err == nil {
		t.Fatal("empty authority revision must fail")
	}
}

func TestBuildV3AuthorityRevisionMetadataUpdatesScrumDirectiveChain(t *testing.T) {
	job := model.Job{
		ID: 9,
		Metadata: json.RawMessage(`{
			"source":"omni-scrum",
			"runtime":"v3",
			"v3_authority_directives":["Fix routing only"]
		}`),
	}
	raw, _, _, err := buildV3AuthorityRevisionMetadata(job, "Also add validation")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "scrum_current_user_instruction") {
		t.Fatalf("removed Scrum directive path survived: %s", raw)
	}
	var metadata struct {
		Directives []string `json:"v3_authority_directives"`
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(metadata.Directives, []string{"Fix routing only", "Also add validation"}) {
		t.Fatalf("directives=%#v", metadata.Directives)
	}
}

func TestV3ScrumCardAuthorityRequiresTypedCardIdentity(t *testing.T) {
	cardID, scrumJob, err := v3ScrumCardAuthority([]byte(`{"source":"omni-scrum","scrum_card_id":"card-7"}`))
	if err != nil || !scrumJob || cardID != "card-7" {
		t.Fatalf("card authority id=%q scrum=%t err=%v", cardID, scrumJob, err)
	}
	if _, _, err := v3ScrumCardAuthority([]byte(`{"source":"omni-scrum"}`)); err == nil {
		t.Fatal("Scrum authority without card identity must fail")
	}
	if _, _, err := v3ScrumCardAuthority([]byte(`{"source":7,"scrum_card_id":"card-7"}`)); err == nil {
		t.Fatal("stringly typed Scrum source must fail")
	}
}

func TestValidatePipelinePreservesEverySupportedPipeline(t *testing.T) {
	for _, pipeline := range []string{
		model.PipelineAssistant,
		model.PipelineChat,
		model.PipelineCoding,
		model.PipelineStory,
		model.PipelineDataQuery,
		model.PipelineDataExplore,
		model.PipelineProjectDebugger,
		model.PipelineScrumCardLLM,
		model.PipelineScrum,
	} {
		got, err := validatePipeline("  " + pipeline + "  ")
		if err != nil {
			t.Fatalf("validatePipeline(%q): %v", pipeline, err)
		}
		if got != pipeline {
			t.Fatalf("validatePipeline(%q)=%q", pipeline, got)
		}
	}
}

func TestValidatePipelineRejectsUnknownInsteadOfFallingBack(t *testing.T) {
	_, err := validatePipeline("mystery")
	if err == nil {
		t.Fatal("unknown pipeline must fail loudly")
	}
	if !errors.Is(err, ErrUnsupportedPipeline) {
		t.Fatalf("error=%v must wrap ErrUnsupportedPipeline", err)
	}
	if got := stepsForPipeline("mystery"); len(got) != 0 {
		t.Fatalf("unknown pipeline produced fallback steps: %#v", got)
	}
}

func TestStepsForPipelineResearchBeforePlan(t *testing.T) {
	tests := []struct {
		name     string
		pipeline string
		want     []string
	}{
		{
			name:     "assistant",
			pipeline: model.PipelineAssistant,
			want: []string{
				"tooling",
				"workspace_scan",
				"tag",
				"retrieve",
				"plan",
				"web_search",
				"analyze",
				"assist",
				"verify",
			},
		},
		{
			name:     "chat",
			pipeline: model.PipelineChat,
			want: []string{
				"tooling",
				"workspace_scan",
				"tag",
				"retrieve",
				"plan",
				"web_search",
				"analyze",
				"roleplay",
				"verify",
			},
		},
		{
			name:     "coding",
			pipeline: model.PipelineCoding,
			want: []string{
				"coding_workflow",
			},
		},
		{
			name:     "story",
			pipeline: model.PipelineStory,
			want: []string{
				"tooling",
				"workspace_scan",
				"tag",
				"retrieve",
				"plan",
				"web_search",
				"analyze",
				"narrate",
				"verify",
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := stepsForPipeline(tc.pipeline)
			if !reflect.DeepEqual(stepActions(got), tc.want) {
				t.Fatalf("stepsForPipeline(%q) actions=%v want %v", tc.pipeline, stepActions(got), tc.want)
			}
			if !strictlyIncreasingSortIndex(got) {
				t.Fatalf("stepsForPipeline(%q) sort indexes must be strictly increasing: %+v", tc.pipeline, got)
			}
		})
	}
}

func TestCodingPipelineIsOnlyCodingWorkflow(t *testing.T) {
	got := stepsForPipeline(model.PipelineCoding)
	want := []stepSeed{{action: "coding_workflow", sortIndex: 5}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stepsForPipeline(coding)=%+v want %+v", got, want)
	}
}

func TestV3LowSignalChatUsesFastPath(t *testing.T) {
	v3Metadata := []byte(`{"runtime":"v3","v3_enabled":true}`)
	got, err := stepsForJob(model.PipelineChat, "hello", v3Metadata)
	if err != nil {
		t.Fatalf("stepsForJob: %v", err)
	}
	want := []stepSeed{{action: "v3_chat_fastpath", sortIndex: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stepsForJob(chat hello,v3)=%+v want %+v", got, want)
	}
}

func TestV3AuthoritativeStepsResearchBeforePlanningAndVerifyBeforeFinalize(t *testing.T) {
	v3Metadata := []byte(`{"runtime":"v3","v3_enabled":true}`)
	got, err := stepsForJob(model.PipelineAssistant, "Research the current API and explain the evidence", v3Metadata)
	if err != nil {
		t.Fatalf("stepsForJob: %v", err)
	}
	want := []string{
		"v3_intent_parse",
		"v3_capability_audit",
		"v3_workspace_research",
		"v3_memory_retrieval",
		"v3_external_research",
		"v3_planning",
		"v3_analysis",
		"v3_response_draft",
		"v3_verification",
		"v3_memory_review",
		"v3_finalize",
	}
	if actions := stepActions(got); !reflect.DeepEqual(actions, want) {
		t.Fatalf("stepsForJob(v3)=%#v want %#v", actions, want)
	}
	if !strictlyIncreasingSortIndex(got) {
		t.Fatalf("v3 sort indexes must increase: %+v", got)
	}
}

func TestCodingJobStepsIgnoreV3Metadata(t *testing.T) {
	v3Metadata := []byte(`{"runtime":"v3","v3_enabled":true,"engine":"native_v3"}`)
	got, err := stepsForJob(model.PipelineCoding, "build app", v3Metadata)
	if err != nil {
		t.Fatalf("stepsForJob: %v", err)
	}
	want := []stepSeed{{action: "coding_workflow", sortIndex: 5}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stepsForJob(coding,v3_metadata)=%+v want %+v", got, want)
	}
}

func TestScrumExternalJobUsesSingleExternalStep(t *testing.T) {
	meta := []byte(`{"source":"omni-scrum","agent_config":{"agent_system":"codex"}}`)
	got, err := stepsForJob("scrum", "implement card", meta)
	if err != nil {
		t.Fatalf("stepsForJob: %v", err)
	}
	want := []stepSeed{{action: "external_agent_execute", sortIndex: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stepsForJob(scrum external)=%+v want %+v", got, want)
	}
}

func TestDataSourceQueryJobUsesSingleQueryStep(t *testing.T) {
	meta, err := datasource.JobMetadata("ds-abc", "Hospital DB", "How many patients checked in today?", "dsc-1")
	if err != nil {
		t.Fatal(err)
	}
	got, err := stepsForJob(model.PipelineDataQuery, "How many patients checked in today?", meta)
	if err != nil {
		t.Fatalf("stepsForJob: %v", err)
	}
	want := []stepSeed{{action: "data_source_query", sortIndex: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stepsForJob(data_query)=%+v want %+v", got, want)
	}
}

func TestScrumOmnidexJobUsesFullAuthoritativeVerificationChain(t *testing.T) {
	meta := []byte(`{"source":"omni-scrum","agent_config":{"agent_system":"omnidex"},"runtime":"v3"}`)
	steps, err := stepsForJob("scrum", "implement card", meta)
	if err != nil {
		t.Fatalf("stepsForJob: %v", err)
	}
	got := stepActions(steps)
	want := []string{
		"v3_intent_parse",
		"v3_capability_audit",
		"v3_workspace_research",
		"v3_memory_retrieval",
		"v3_external_research",
		"v3_planning",
		"v3_analysis",
		"v3_response_draft",
		"v3_verification",
		"v3_memory_review",
		"v3_finalize",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stepsForJob(scrum omnidex)=%#v want %#v", got, want)
	}
}

func TestStepsForJobRejectsRemovedExecutionAgentMetadata(t *testing.T) {
	_, err := stepsForJob(model.PipelineChat, "hello", []byte(`{"execution_agent":"cursor"}`))
	if err == nil {
		t.Fatal("expected removed execution_agent metadata to fail")
	}
}

func stepActions(steps []stepSeed) []string {
	out := make([]string, 0, len(steps))
	for _, step := range steps {
		out = append(out, step.action)
	}
	return out
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func strictlyIncreasingSortIndex(steps []stepSeed) bool {
	if len(steps) < 2 {
		return true
	}
	last := steps[0].sortIndex
	for _, step := range steps[1:] {
		if step.sortIndex <= last {
			return false
		}
		last = step.sortIndex
	}
	return true
}
