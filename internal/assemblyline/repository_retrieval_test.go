package assemblyline

import (
	"reflect"
	"strings"
	"testing"
)

func TestRepositoryRetrievalDecisionIsGroundedAndPathBlind(t *testing.T) {
	t.Parallel()

	input := RepositoryRetrievalInput{
		ResearchNeed: "Find every direct reference to ApplyResponseCorrection before changing its contract.",
	}
	decision := RepositoryRetrievalDecision{
		Schema: RepositoryRetrievalSchemaV1, Operation: RetrievalDirectReferences,
		QueryQuote: "ApplyResponseCorrection",
	}
	if err := decision.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	for name, invalid := range map[string]RepositoryRetrievalDecision{
		"paraphrase": {Schema: RepositoryRetrievalSchemaV1, Operation: RetrievalDirectReferences, QueryQuote: "response correction callers"},
		"operation":  {Schema: RepositoryRetrievalSchemaV1, Operation: RepositoryRetrievalOperation("shell"), QueryQuote: "ApplyResponseCorrection"},
		"schema":     {Schema: "wrong", Operation: RetrievalDirectReferences, QueryQuote: "ApplyResponseCorrection"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := invalid.ValidateFor(input); err == nil {
				t.Fatalf("accepted invalid decision %#v", invalid)
			}
		})
	}
	for _, typ := range []reflect.Type{
		reflect.TypeOf(RepositoryRetrievalInput{}), reflect.TypeOf(RepositoryRetrievalDecision{}),
	} {
		for _, forbidden := range []string{"Path", "File", "Tree", "Command", "SQL", "Shell", "Workspace"} {
			if _, exists := typ.FieldByName(forbidden); exists {
				t.Fatalf("%s exposes forbidden field %q", typ.Name(), forbidden)
			}
		}
	}
}

func TestRepositoryRetrievalPortableProtocolKeepsAdviserUnstructured(t *testing.T) {
	t.Parallel()

	input := RepositoryRetrievalInput{
		ResearchNeed: "Locate the declaration of ParseEnvelope so its validation boundary can be reviewed.",
	}
	direct, err := NewRepositoryRetrievalJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(direct)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, input.ResearchNeed) || schema["type"] != "object" {
		t.Fatalf("direct prompt=%q schema=%#v", prompt, schema)
	}

	briefing, err := NewRepositoryRetrievalBriefingJob(input)
	if err != nil {
		t.Fatal(err)
	}
	briefingPrompt, briefingSchema, err := RenderPortableJob(briefing)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(briefingPrompt, "code-registered retrieval lens") || briefingSchema["type"] != "object" {
		t.Fatalf("briefing prompt=%q schema=%#v", briefingPrompt, briefingSchema)
	}

	advisory, err := NewRepositoryRetrievalAdvisoryJob(RepositoryRetrievalAdvisoryInput{
		Original: input, Lens: RetrievalLensRelationDirection,
	})
	if err != nil {
		t.Fatal(err)
	}
	advisoryPrompt, advisorySchema, err := RenderPortableJob(advisory)
	if err != nil {
		t.Fatal(err)
	}
	if advisorySchema != nil || !strings.Contains(advisoryPrompt, "plain text") || strings.Contains(advisoryPrompt, "RESPONSE_SCHEMA") {
		t.Fatalf("advisory prompt=%q schema=%#v", advisoryPrompt, advisorySchema)
	}

	synthesis, err := NewRepositoryRetrievalSynthesisJob(RepositoryRetrievalSynthesisInput{
		Original: input, AdvisoryMemo: "The need asks for incoming references, not the declaration itself.",
	})
	if err != nil {
		t.Fatal(err)
	}
	synthesisPrompt, synthesisSchema, err := RenderPortableJob(synthesis)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"original prompt is authoritative", "UNTRUSTED_ADVISORY_MEMO_JSON", input.ResearchNeed} {
		if !strings.Contains(synthesisPrompt, required) {
			t.Fatalf("synthesis prompt missing %q:\n%s", required, synthesisPrompt)
		}
	}
	if synthesisSchema["type"] != "object" {
		t.Fatalf("synthesis schema=%#v", synthesisSchema)
	}
}

func TestRepositoryRetrievalRejectsUnregisteredLensAndOversizedMemo(t *testing.T) {
	t.Parallel()

	input := RepositoryRetrievalInput{ResearchNeed: "Find the code responsible for bounded retry accounting."}
	if _, err := NewRepositoryRetrievalAdvisoryJob(RepositoryRetrievalAdvisoryInput{
		Original: input, Lens: RepositoryRetrievalLens("invented"),
	}); err == nil {
		t.Fatal("unregistered retrieval lens was accepted")
	}
	if _, err := NewRepositoryRetrievalSynthesisJob(RepositoryRetrievalSynthesisInput{
		Original: input, AdvisoryMemo: strings.Repeat("x", maxRepositoryRetrievalMemoBytes+1),
	}); err == nil {
		t.Fatal("oversized retrieval memo was accepted")
	}
}
