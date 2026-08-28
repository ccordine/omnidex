package assemblyline

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestConversationObjectiveKindStationHasOneExactSemanticResponsibility(t *testing.T) {
	t.Parallel()

	instruction := "  Explain how the scheduler and dispatcher relate.\n"
	input := ConversationObjectiveKindInput{ExactInstruction: instruction}
	job, err := NewConversationObjectiveKindJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkConversationObjectiveKind {
		t.Fatalf("kind=%q", job.Kind)
	}

	var decoded ConversationObjectiveKindInput
	if err := json.Unmarshal(job.Payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ExactInstruction != instruction {
		t.Fatalf("instruction changed: got %q want %q", decoded.ExactInstruction, instruction)
	}

	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{instruction, string(ObjectiveKindRepositoryRead)} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("prompt omitted %q:\n%s", required, prompt)
		}
	}
	assertExactJSONFields(t, reflect.TypeOf(input), []string{"exact_instruction", "objective_context", "database_evidence_available"})
	assertExactJSONFields(t, reflect.TypeOf(ConversationObjectiveKindDecision{}), []string{"schema", "kind"})

	encoded, err := json.Marshal(job.Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"path", "tool", "capabilit", "plan", "completion"} {
		if strings.Contains(strings.ToLower(string(encoded)), `"`+forbidden) {
			t.Fatalf("objective-kind payload exposes forbidden field %q: %s", forbidden, encoded)
		}
	}
}

func TestConversationObjectiveKindProjectsOnlyMinifiedContextCapsule(t *testing.T) {
	t.Parallel()
	input := ConversationObjectiveKindInput{
		ExactInstruction: "Do that.",
		Context: minifiedObjectiveContext(
			"The requested action is to replace the parser-owned cache implementation.",
		),
	}
	prompt, err := BuildConversationObjectiveKindPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "replace the parser-owned cache implementation") {
		t.Fatalf("minified context capsule missing from prompt: %s", prompt)
	}
	for _, forbidden := range []string{
		"user_authorities", "assistant_results", `"message_id"`, `"job_id"`,
		`"candidate_id"`, `"content_sha256"`, `"namespace"`,
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("raw context field %q leaked into prompt: %s", forbidden, prompt)
		}
	}
	input.Context.Capsules[0].Content = "\x00"
	if _, err := NewConversationObjectiveKindJob(input); err == nil {
		t.Fatal("invalid minified context capsule was accepted")
	}
}

func TestConversationObjectiveKindPromptDefinesConversationalAndOperationalBoundaries(t *testing.T) {
	t.Parallel()
	prompt, err := BuildConversationObjectiveKindPrompt(ConversationObjectiveKindInput{
		ExactInstruction: "Hello there.",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"including greetings and small talk",
		"requires inspecting an existing repository",
		"requires changing a workspace",
		"explicit web-search or research request",
		"only when the instruction requires",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("objective-kind prompt omits boundary %q:\n%s", required, prompt)
		}
	}
}

func TestConversationObjectiveKindAcceptsOnlyRegisteredKinds(t *testing.T) {
	t.Parallel()

	input := ConversationObjectiveKindInput{ExactInstruction: "Tell me a story."}
	for _, kind := range []ConversationObjectiveKind{
		ObjectiveKindAnswer,
		ObjectiveKindRepositoryRead,
		ObjectiveKindWorkspaceMutation,
		ObjectiveKindExternalAnswer,
		ObjectiveKindStory,
	} {
		decision := ConversationObjectiveKindDecision{
			Schema: ConversationObjectiveKindSchemaV1,
			Kind:   kind,
		}
		if err := decision.ValidateFor(input); err != nil {
			t.Fatalf("registered kind %q rejected: %v", kind, err)
		}
	}
	database := ConversationObjectiveKindDecision{Schema: ConversationObjectiveKindSchemaV1, Kind: ObjectiveKindDatabaseRead}
	if err := database.ValidateFor(input); err == nil {
		t.Fatal("database objective accepted without an explicit source binding")
	}
	input.DatabaseEvidenceAvailable = true
	if err := database.ValidateFor(input); err != nil {
		t.Fatalf("database objective rejected with source binding: %v", err)
	}
	for _, decision := range []ConversationObjectiveKindDecision{
		{Schema: "wrong", Kind: ObjectiveKindStory},
		{Schema: ConversationObjectiveKindSchemaV1},
		{Schema: ConversationObjectiveKindSchemaV1, Kind: "research_agent"},
	} {
		if err := decision.ValidateFor(input); err == nil {
			t.Fatalf("invalid decision accepted: %#v", decision)
		}
	}
}

func TestConversationObjectiveKindRejectsMalformedAuthority(t *testing.T) {
	t.Parallel()

	invalidUTF8 := string([]byte{0xff})
	if utf8.ValidString(invalidUTF8) {
		t.Fatal("invalid UTF-8 fixture is valid")
	}
	for name, instruction := range map[string]string{
		"empty":        "",
		"blank":        " \n\t ",
		"nul":          "answer\x00this",
		"invalid_utf8": invalidUTF8,
		"oversized":    strings.Repeat("x", maxConversationInstructionBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewConversationObjectiveKindJob(ConversationObjectiveKindInput{
				ExactInstruction: instruction,
			}); err == nil {
				t.Fatalf("malformed instruction %q was accepted", name)
			}
		})
	}
}

func TestConversationObjectiveKindDecodeIsExact(t *testing.T) {
	t.Parallel()

	input := ConversationObjectiveKindInput{ExactInstruction: "Inspect the repository."}
	valid := "repository_read"
	decision, err := DecodeConversationObjectiveKindDecision(input, valid)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Kind != ObjectiveKindRepositoryRead {
		t.Fatalf("decision=%#v", decision)
	}
	for _, raw := range []string{
		`{"kind":"repository_read","tool":"grep"}`,
		`"repository_read"`,
		"kind: repository_read",
		"unknown",
		valid + " answer",
	} {
		if _, err := DecodeConversationObjectiveKindDecision(input, raw); err == nil {
			t.Fatalf("invalid raw decision accepted: %s", raw)
		}
	}
	if _, err := DecodeConversationObjectiveKindDecision(input, strings.Repeat("x", maxPortableCandidateBytes+1)); err == nil {
		t.Fatal("oversized objective-kind candidate was parsed")
	}
}

func assertExactJSONFields(t *testing.T, value reflect.Type, want []string) {
	t.Helper()
	if value.NumField() != len(want) {
		t.Fatalf("%s exposes %d fields, want %d", value.Name(), value.NumField(), len(want))
	}
	for index, name := range want {
		got := strings.Split(value.Field(index).Tag.Get("json"), ",")[0]
		if got != name {
			t.Fatalf("%s field %d JSON name=%q want %q", value.Name(), index, got, name)
		}
	}
}

func assertExactObjectSchemaFields(t *testing.T, schema map[string]any, want []string) {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties=%#v", schema["properties"])
	}
	required, ok := schema["required"].([]string)
	if !ok || !reflect.DeepEqual(required, want) {
		t.Fatalf("schema required=%#v want %#v", schema["required"], want)
	}
	if len(properties) != len(want) || schema["additionalProperties"] != false {
		t.Fatalf("schema boundary=%#v", schema)
	}
	for _, name := range want {
		if _, exists := properties[name]; !exists {
			t.Fatalf("schema omits property %q: %#v", name, schema)
		}
	}
}
