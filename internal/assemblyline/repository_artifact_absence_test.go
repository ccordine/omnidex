package assemblyline

import (
	"strings"
	"testing"
)

func TestRepositoryArtifactAbsenceAsksOnlyOneRelation(t *testing.T) {
	t.Parallel()
	input := RepositoryArtifactAbsenceInput{
		RequirementQuote: "The known Go artifact declaring func Obsolete() int and all behavior it owns must no longer exist.",
	}
	job, err := NewRepositoryArtifactAbsenceJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkRepositoryArtifactAbsence {
		t.Fatalf("work kind=%q", job.Kind)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		input.RequirementQuote,
		string(RepositoryArtifactMustBeAbsent),
		string(RepositoryArtifactAbsenceNotExplicit),
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("repository artifact absence envelope omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"plain_text", "create_file", "delete_file", "write_file", "shell command",
		"filename", "accept", "reject", "retry",
	} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("repository artifact absence envelope exposed forbidden authority %q", forbidden)
		}
	}
}

func TestRepositoryArtifactAbsenceAcceptsOnlyClosedRawRelation(t *testing.T) {
	t.Parallel()
	input := RepositoryArtifactAbsenceInput{
		RequirementQuote: "One known semantic artifact must no longer exist.",
	}
	for _, relation := range []RepositoryArtifactAbsenceRelation{
		RepositoryArtifactMustBeAbsent, RepositoryArtifactAbsenceNotExplicit,
	} {
		decision := RepositoryArtifactAbsenceDecision{
			Schema: RepositoryArtifactAbsenceSchemaV1, Relation: relation,
		}
		if err := decision.ValidateFor(input); err != nil {
			t.Fatalf("relation %q: %v", relation, err)
		}
		decoded, err := DecodeRepositoryArtifactAbsenceDecision(input, string(relation))
		if err != nil || decoded != decision {
			t.Fatalf("decoded=%+v want=%+v err=%v", decoded, decision, err)
		}
	}
	if _, err := DecodeRepositoryArtifactAbsenceDecision(
		input, `{"relation":"repository_artifact_must_be_absent"}`,
	); err == nil {
		t.Fatal("accepted JSON wrapper")
	}
	if _, err := DecodeRepositoryArtifactAbsenceDecision(input, "delete_file"); err == nil {
		t.Fatal("accepted model-authored operation")
	}
}

func TestRepositoryArtifactAbsenceRejectsPhysicalArtifactQuote(t *testing.T) {
	t.Parallel()
	for _, quote := range []string{
		"./obsolete.go must no longer exist.",
		"Remove internal/legacy/adapter.",
	} {
		if _, err := NewRepositoryArtifactAbsenceJob(
			RepositoryArtifactAbsenceInput{RequirementQuote: quote},
		); err == nil {
			t.Fatalf("physical quote accepted: %q", quote)
		}
	}
}
