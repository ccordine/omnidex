package assemblyline

import (
	"strings"
	"testing"
)

func TestPlainTextArtifactCreationAsksOnlyOneRelation(t *testing.T) {
	t.Parallel()
	input := PlainTextArtifactCreationInput{
		RequirementQuote: "Create ARTIFACT_1 containing the complete note: Release ready.",
	}
	job, err := NewPlainTextArtifactCreationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkPlainTextArtifactCreation {
		t.Fatalf("work kind=%q", job.Kind)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		input.RequirementQuote,
		string(OneNewCompletePlainTextArtifactRequired),
		string(PlainTextArtifactCreationNotExplicit),
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("plain-text artifact creation envelope omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"must_be_absent", "create_file", "write_file", "shell command",
		"filename", "accept", "reject", "retry",
	} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("plain-text artifact creation envelope exposed forbidden authority %q", forbidden)
		}
	}
}

func TestPlainTextArtifactCreationAcceptsOnlyClosedRawRelation(t *testing.T) {
	t.Parallel()
	input := PlainTextArtifactCreationInput{
		RequirementQuote: "Create ARTIFACT_1 containing the complete note: Release ready.",
	}
	for _, relation := range []PlainTextArtifactCreationRelation{
		OneNewCompletePlainTextArtifactRequired, PlainTextArtifactCreationNotExplicit,
	} {
		decision := PlainTextArtifactCreationDecision{
			Schema: PlainTextArtifactCreationSchemaV1, Relation: relation,
		}
		if err := decision.ValidateFor(input); err != nil {
			t.Fatalf("relation %q: %v", relation, err)
		}
		decoded, err := DecodePlainTextArtifactCreationDecision(input, string(relation))
		if err != nil || decoded != decision {
			t.Fatalf("decoded=%+v want=%+v err=%v", decoded, decision, err)
		}
	}
	if _, err := DecodePlainTextArtifactCreationDecision(
		input, `{"relation":"one_new_complete_plain_text_artifact_required"}`,
	); err == nil {
		t.Fatal("accepted JSON wrapper")
	}
	if _, err := DecodePlainTextArtifactCreationDecision(input, "create_file"); err == nil {
		t.Fatal("accepted model-authored operation")
	}
}

func TestPlainTextArtifactCreationRejectsPhysicalArtifactQuote(t *testing.T) {
	t.Parallel()
	if _, err := NewPlainTextArtifactCreationJob(PlainTextArtifactCreationInput{
		RequirementQuote: "Create notes/release.txt containing Release ready.",
	}); err == nil {
		t.Fatal("physical quote accepted")
	}
}
