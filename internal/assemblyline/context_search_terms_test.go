package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestContextSearchTermsGreetingReturnsExplicitEmptySet(t *testing.T) {
	t.Parallel()
	input := ContextSearchTermsInput{ExactInstruction: "Hello."}
	job, err := NewContextSearchTermsJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkContextSearchTerms {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `"exact_instruction":"Hello."`) {
		t.Fatalf("prompt lost exact instruction: %s", prompt)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 1 || payload["exact_instruction"] == nil {
		t.Fatalf("search-term payload contains context beyond the exact instruction: %s", job.Payload)
	}
	for _, forbidden := range []string{"candidate_authorities", "provider", "tool", "memory_namespace"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("search-term station received forbidden %q context: %s", forbidden, prompt)
		}
	}
	if schema["additionalProperties"] != false {
		t.Fatal("response schema permits additional fields")
	}
	decision, err := DecodeContextSearchTermsDecision(
		input,
		fmt.Sprintf(`{"schema":%q,"terms":[]}`, ContextSearchTermsSchemaV1),
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Terms == nil || len(decision.Terms) != 0 {
		t.Fatalf("greeting terms=%#v, want explicit empty set", decision.Terms)
	}
}

func TestContextSearchTermsAnaphoricInstructionHasOnlyBoundedConceptLeaves(t *testing.T) {
	t.Parallel()
	input := ContextSearchTermsInput{ExactInstruction: "  Do it again.\n"}
	job, err := NewContextSearchTermsJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, _, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, `"exact_instruction":"  Do it again.\n"`) {
		t.Fatalf("exact anaphoric instruction changed: %s", prompt)
	}
	raw := fmt.Sprintf(
		`{"schema":%q,"terms":["previous action to repeat","most recent requested operation"]}`,
		ContextSearchTermsSchemaV1,
	)
	decision, err := DecodeContextSearchTermsDecision(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.Terms) != 2 || decision.Terms[0] != "previous action to repeat" {
		t.Fatalf("decision=%#v", decision)
	}
}

func TestContextSearchTermsRejectsMalformedOrUnboundedLeaves(t *testing.T) {
	t.Parallel()
	input := ContextSearchTermsInput{ExactInstruction: "Repeat the last operation."}
	schema := fmt.Sprintf(`"schema":%q`, ContextSearchTermsSchemaV1)
	tests := map[string]string{
		"null set":       `{` + schema + `,"terms":null}`,
		"duplicate term": `{` + schema + `,"terms":["prior action","PRIOR ACTION"]}`,
		"too many":       `{` + schema + `,"terms":["one","two","three","four"]}`,
		"untrimmed":      `{` + schema + `,"terms":[" prior action"]}`,
		"multiline":      `{` + schema + `,"terms":["prior\naction"]}`,
		"oversized":      `{` + schema + `,"terms":["` + strings.Repeat("x", MaxContextSearchTermBytes+1) + `"]}`,
		"unknown field":  `{` + schema + `,"terms":[],"source":"memory"}`,
		"trailing":       `{` + schema + `,"terms":[]} {}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeContextSearchTermsDecision(input, raw); err == nil {
				t.Fatal("invalid search-term leaf was accepted")
			}
		})
	}
	for name, instruction := range map[string]string{
		"blank": " \n", "nul": "hello\x00", "invalid UTF-8": string([]byte{0xff}),
		"oversized": strings.Repeat("x", maxConversationInstructionBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewContextSearchTermsJob(ContextSearchTermsInput{ExactInstruction: instruction}); err == nil {
				t.Fatal("invalid exact instruction was accepted")
			}
		})
	}
}
