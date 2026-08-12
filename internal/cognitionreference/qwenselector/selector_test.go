package qwenselector

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreference"
	"github.com/gryph/omnidex/internal/llm"
)

func TestSelectorSendsOnlyOneExactSemanticGapAndAcceptsOneCandidateID(t *testing.T) {
	t.Parallel()
	gap := testGap()
	client, selector := testSelector(t, `{"candidate_id":"C17"}`, Limits{3000, 64})

	selected, err := selector.Select(t.Context(), gap)
	if err != nil {
		t.Fatal(err)
	}
	if selected != "C17" || client.calls != 1 {
		t.Fatalf("selection=%q calls=%d, want C17 and exactly one generation", selected, client.calls)
	}
	wantPrompt := `{"candidates":[{"candidate_id":"C17","evidence_ids":["E10","E20"],"summary":"Retain the sheltered interpretation."},{"candidate_id":"C23","evidence_ids":["E10","E20"],"summary":"Retain the exposed interpretation."}],"evidence":[{"content":"The clue permits either interpretation.","id":"E10"},{"content":"Both candidates are legal, equal-cost, and equally supported.","id":"E20"}],"instruction":"Select exactly one candidate_id from candidates. Return no other field.","question":"Which equally supported interpretation should be retained?","schema":"omnidex.semantic-gap-selection.v1"}`
	if client.prepared.Prompt != wantPrompt {
		t.Fatalf("model-visible prompt=\n%s\nwant=\n%s", client.prepared.Prompt, wantPrompt)
	}
	input, err := llm.ExactPreparedModelInput(client.prepared.Prompt, client.prepared.PromptHint)
	if err != nil {
		t.Fatal(err)
	}
	if input != wantPrompt+"\n"+llm.MinimalGeneratePrompt {
		t.Fatalf("exact model input=%q", input)
	}
	wantSchema := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []any{"candidate_id"},
		"properties": map[string]any{
			"candidate_id": map[string]any{"type": "string", "enum": []any{"C17", "C23"}},
		},
	}
	if !reflect.DeepEqual(client.prepared.ResponseSchema, wantSchema) {
		t.Fatalf("response schema=%#v, want %#v", client.prepared.ResponseSchema, wantSchema)
	}
	var exactRequest map[string]any
	if err := json.Unmarshal(client.request, &exactRequest); err != nil {
		t.Fatal(err)
	}
	wantRequestKeys := []string{"format", "model", "options", "prompt", "raw", "shift", "stream", "think", "truncate"}
	gotRequestKeys := make([]string, 0, len(exactRequest))
	for key := range exactRequest {
		gotRequestKeys = append(gotRequestKeys, key)
	}
	slices.Sort(gotRequestKeys)
	if !reflect.DeepEqual(gotRequestKeys, wantRequestKeys) {
		t.Fatalf("exact request keys=%v, want %v", gotRequestKeys, wantRequestKeys)
	}
	if exactRequest["model"] != testModel || exactRequest["prompt"] != input ||
		exactRequest["raw"] != true || exactRequest["stream"] != false ||
		exactRequest["think"] != false || exactRequest["shift"] != false ||
		exactRequest["truncate"] != false ||
		!reflect.DeepEqual(exactRequest["format"], wantSchema) {
		t.Fatalf("exact provider request changed frozen fields: %#v", exactRequest)
	}
	wantOptions := map[string]any{
		"num_ctx": float64(32768), "num_predict": float64(64), "temperature": float64(0),
	}
	if !reflect.DeepEqual(exactRequest["options"], wantOptions) {
		t.Fatalf("exact request options=%#v, want %#v", exactRequest["options"], wantOptions)
	}
	for _, forbiddenKey := range []string{"messages", "tools", "tool_choice", "functions", "actions"} {
		if _, exists := exactRequest[forbiddenKey]; exists {
			t.Fatalf("exact provider request exposes forbidden key %q", forbiddenKey)
		}
	}
	if client.prepared.ThinkingEnabled || client.prepared.Temperature == nil ||
		*client.prepared.Temperature != 0 || client.prepared.MaxOutputTokens != 64 {
		t.Fatalf("prepared model changed exact deterministic sampling: %#v", client.prepared)
	}
	for _, forbidden := range []string{
		`"action"`, `"operation"`, `"tool"`, `"argument"`, `"evidence_refs"`,
		`"expected_effect"`, `"proposal"`, `"attention"`, `"objective_id"`, `"gap_id"`,
		`"gap_kind"`,
	} {
		if strings.Contains(string(client.request), forbidden) {
			t.Fatalf("exact provider request exposes forbidden field %s: %s", forbidden, client.request)
		}
	}
}

func TestSelectorRejectsEveryInexactOrOutOfGapResponseWithoutFallback(t *testing.T) {
	t.Parallel()
	for name, content := range map[string]string{
		"empty object":      `{}`,
		"empty candidate":   `{"candidate_id":""}`,
		"unknown candidate": `{"candidate_id":"C99"}`,
		"duplicate field":   `{"candidate_id":"C17","candidate_id":"C23"}`,
		"unknown field":     `{"candidate_id":"C17","reason":"invented"}`,
		"inexact alias":     `{"Candidate_ID":"C17"}`,
		"trailing object":   `{"candidate_id":"C17"}{}`,
		"non-object":        `"C17"`,
	} {
		name, content := name, content
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client, selector := testSelector(t, content, Limits{3000, 64})
			selected, err := selector.Select(t.Context(), testGap())
			if err == nil || selected != "" || client.calls != 1 {
				t.Fatalf("selection=%q error=%v calls=%d, want loud one-call rejection", selected, err, client.calls)
			}
		})
	}
}

func TestSelectorFailsBeforeGenerationWhenContextOrInputBudgetIsUnavailable(t *testing.T) {
	t.Parallel()
	client, selector := testSelector(t, `{"candidate_id":"C17"}`, Limits{1, 64})
	if _, err := selector.Select(t.Context(), testGap()); !errors.Is(err, ErrSelection) {
		t.Fatalf("input budget error=%v, want ErrSelection", err)
	}
	if client.calls != 0 {
		t.Fatalf("budget failure generated %d times", client.calls)
	}

	client, selector = testSelector(t, `{"candidate_id":"C17"}`, Limits{3000, 64})
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := selector.Select(canceled, testGap()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error=%v, want context.Canceled", err)
	}
	if client.calls != 0 {
		t.Fatalf("canceled selection generated %d times", client.calls)
	}
}

func testSelector(t *testing.T, content string, limits Limits) (*recordingExactClient, *Selector) {
	t.Helper()
	provider := testProvider(t)
	client := &recordingExactClient{provider: provider, content: content}
	selector, err := New(client, provider, limits)
	if err != nil {
		t.Fatal(err)
	}
	return client, selector
}

func testGap() cognitionreference.SemanticGap {
	return cognitionreference.SemanticGap{
		ID: "gap.route", Kind: cognitionreference.GapCandidateSelection,
		ObjectiveID: "objective.route",
		Question:    "Which equally supported interpretation should be retained?",
		Evidence: []cognitionreference.SemanticEvidence{
			{ID: "E10", Content: "The clue permits either interpretation."},
			{ID: "E20", Content: "Both candidates are legal, equal-cost, and equally supported."},
		},
		Candidates: []cognitionreference.SemanticCandidate{
			{ID: "C17", Summary: "Retain the sheltered interpretation.", EvidenceIDs: []cognitionreference.EvidenceID{"E10", "E20"}},
			{ID: "C23", Summary: "Retain the exposed interpretation.", EvidenceIDs: []cognitionreference.EvidenceID{"E10", "E20"}},
		},
	}
}
