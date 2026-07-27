package worker

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialists"
)

func TestV3SpecialistSemanticValidationRejectsSchemaValidOutput(t *testing.T) {
	spec := specialists.Spec{ID: "prompt_interpreter", Purpose: "interpret one instruction"}
	wantErr := errors.New("objective hierarchy is invalid")
	raw := `{
		"contract_version":"1.0",
		"role_id":"prompt_interpreter",
		"status":"success",
		"output":{"goal":"build an app"}
	}`

	_, _, err := decodeAndValidateV3SpecialistResponse(raw, "prompt_interpreter", spec, func(output map[string]any) error {
		if output["goal"] != "build an app" {
			t.Fatalf("validator received output=%#v", output)
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("decodeAndValidateV3SpecialistResponse() error=%v, want %v", err, wantErr)
	}
}

func TestV3SpecialistSemanticValidationAcceptsCorrectedOutput(t *testing.T) {
	spec := specialists.Spec{ID: "prompt_interpreter", Purpose: "interpret one instruction"}
	raw := `{
		"contract_version":"1.0",
		"role_id":"prompt_interpreter",
		"status":"success",
		"output":{"goal":"build an app"}
	}`

	output, _, err := decodeAndValidateV3SpecialistResponse(raw, "prompt_interpreter", spec, func(output map[string]any) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if output["goal"] != "build an app" {
		t.Fatalf("output=%#v", output)
	}
}

func TestV3SpecialistPromptEndsWithAuthoritativeExecutionCommand(t *testing.T) {
	prompt, err := buildV3SpecialistPrompt(
		"Interpret the typed request.",
		json.RawMessage(`{"type":"object"}`),
		v3SpecialistInvocation{RoleID: "prompt_interpreter"},
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "CONTROL_PLANE_COMMAND: Execute the SPECIALIST_INVOCATION above now. Return exactly one raw JSON response envelope beginning with { and ending with }. Do not use markdown fences, acknowledge the command, or discuss the request."
	if !strings.HasSuffix(prompt, want) {
		t.Fatalf("specialist prompt must end with authoritative activation command; suffix=%q", prompt[max(0, len(prompt)-len(want)-40):])
	}
}

func TestV3SpecialistPromptUsesUnambiguousSuccessEnvelope(t *testing.T) {
	prompt, err := buildV3SpecialistPrompt(
		"Plan the typed objective.",
		json.RawMessage(`{"type":"object"}`),
		v3SpecialistInvocation{RoleID: "executive_planner"},
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`SUCCESS_ENVELOPE: {"contract_version":"1.0","role_id":"executive_planner","status":"success","output":<OUTPUT_SCHEMA>}`,
		"For success, omit `error` entirely; never emit it as {}, null, or an empty object.",
		`FAILURE_ENVELOPE: {"contract_version":"1.0","role_id":"executive_planner","status":"blocked|fail","output":{},"error":{"code":"...","message":"...","retryable":false}}`,
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("specialist prompt missing %q", required)
		}
	}
	if strings.Contains(prompt, `"status":"success|blocked|fail"`) {
		t.Fatal("specialist prompt retained the ambiguous combined envelope")
	}
}

func TestRuntimeV3ExternalResearchPrefersSearchQueryMetadata(t *testing.T) {
	job := model.Job{Instruction: "long instruction", Metadata: json.RawMessage(`{"search_query":"focused docs query"}`)}
	if got := externalResearchQuery(job, artifacts.IntentArtifact{}); got != "focused docs query" {
		t.Fatalf("query=%q", got)
	}
}

func TestGenericNonAnswerDetectsClarificationTemplates(t *testing.T) {
	cases := []string{
		"Understood! Please let me know what specific output you need.",
		"Sure, I can do that. What is the output you need?",
		"Could you clarify what you want?",
		"Understood. I will return only the requested output as per your instructions. If you have any questions or need further assistance, feel free to ask!",
		"Sure, please provide me with the details of what you need the output to be.",
		"Sure, please specify what you need me to return.",
		"Sure, please provide the details of what you need me to return.",
	}
	for _, tc := range cases {
		if !genericNonAnswer(tc) {
			t.Fatalf("genericNonAnswer(%q)=false, want true", tc)
		}
	}
	if genericNonAnswer("Tokio's spawn_blocking is intended for blocking operations; CPU-bound work may need a separate executor.") {
		t.Fatalf("genericNonAnswer rejected a substantive response")
	}
}

func TestCollectSubtaskResultsPreservesTypedRoutingContext(t *testing.T) {
	stored, err := json.Marshal(artifacts.SubtaskResultArtifact{
		SubtaskID: "t0", Kind: artifacts.SubtaskKindResearch, RoleID: "workspace_researcher", ObjectiveID: "inspect", Objective: "Inspect stored state", Priority: 100, Summary: "stored subtask output", Sources: []string{"workspace"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := json.Marshal(artifacts.SubtaskResultArtifact{
		SubtaskID: "t1", Kind: artifacts.SubtaskKindResearch, RoleID: "web_researcher", ObjectiveID: "research", Objective: "Research current state", Priority: 90, Summary: "fresh subtask output", Sources: []string{"web_search"},
	})
	if err != nil {
		t.Fatal(err)
	}
	rt := &nativeRuntimeV3{
		contexts: map[string]string{"subtask:t1": string(fresh)},
		claim: &model.ClaimedStep{Contexts: []model.StepContext{
			{Key: "subtask:t0", Value: string(stored)},
			{Key: "analysis", Value: "not a subtask"},
		}},
	}
	got, err := rt.collectSubtaskResults()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("subtask outputs=%#v, want 2", got)
	}
	if got[0].RoleID != "workspace_researcher" || got[1].RoleID != "web_researcher" {
		t.Fatalf("typed role routing was lost: %#v", got)
	}
}
