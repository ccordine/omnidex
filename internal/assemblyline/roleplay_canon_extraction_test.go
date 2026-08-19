package assemblyline

import (
	"strings"
	"testing"
)

func TestRoleplayCanonExtractionReturnsOnlyNewBoundedFacts(t *testing.T) {
	input := RoleplayCanonExtractionInput{
		ExactInstruction:  "Continue the scene.",
		AssistantResponse: "Rain began over the harbor as Bob closed the west gate.",
		KnownFacts:        []string{"Bob is at the harbor."},
	}
	job, err := NewRoleplayCanonExtractionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, input.AssistantResponse) || schema == nil {
		t.Fatalf("prompt=%q schema=%#v", prompt, schema)
	}
	decision, err := DecodeRoleplayCanonExtractionDecision(input,
		`{"schema":"omnidex.roleplay-canon-extraction.v1","facts":["Rain began over the harbor.","Bob closed the west gate."]}`,
	)
	if err != nil || len(decision.Facts) != 2 {
		t.Fatalf("decision=%#v err=%v", decision, err)
	}
	for _, invalid := range []RoleplayCanonExtractionDecision{
		{Schema: RoleplayCanonExtractionSchemaV1, Facts: nil},
		{Schema: RoleplayCanonExtractionSchemaV1, Facts: []string{"Bob is at the harbor."}},
		{Schema: RoleplayCanonExtractionSchemaV1, Facts: []string{"Same.", "Same."}},
	} {
		if err := invalid.ValidateFor(input); err == nil {
			t.Fatalf("invalid extraction accepted: %#v", invalid)
		}
	}
}
