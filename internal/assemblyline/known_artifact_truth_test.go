package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestKnownArtifactTruthClassifiesOnlyDesiredTruth(t *testing.T) {
	t.Parallel()
	input := KnownArtifactTruthInput{
		RequirementQuote: "The known Go artifact declaring func Obsolete() int must no longer exist.",
	}
	job, err := NewKnownArtifactTruthJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkKnownArtifactTruth {
		t.Fatalf("work kind=%q", job.Kind)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	rawSchema, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		input.RequirementQuote, string(KnownArtifactMustBeAbsent),
		string(KnownArtifactTruthNotApplicable), KnownArtifactTruthSchemaV1,
	} {
		if !strings.Contains(prompt+string(rawSchema), required) {
			t.Fatalf("known artifact truth envelope omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"create_file", "delete_file", "write_file", "rename_file", "move_file",
		"filesystem operation", "shell command", "path", "filename",
	} {
		if strings.Contains(strings.ToLower(prompt+string(rawSchema)), forbidden) {
			t.Fatalf("known artifact truth envelope exposed forbidden authority %q", forbidden)
		}
	}
}

func TestKnownArtifactTruthAcceptsOnlyClosedDesiredTruth(t *testing.T) {
	t.Parallel()
	input := KnownArtifactTruthInput{RequirementQuote: "An obsolete semantic artifact must no longer exist."}
	for _, truth := range []KnownArtifactTruth{
		KnownArtifactMustBeAbsent, KnownArtifactTruthNotApplicable,
	} {
		decision := KnownArtifactTruthDecision{Schema: KnownArtifactTruthSchemaV1, Truth: truth}
		if err := decision.ValidateFor(input); err != nil {
			t.Fatalf("truth %q: %v", truth, err)
		}
	}
	for _, decision := range []KnownArtifactTruthDecision{
		{Schema: "wrong", Truth: KnownArtifactMustBeAbsent},
		{Schema: KnownArtifactTruthSchemaV1, Truth: "delete_file"},
		{Schema: KnownArtifactTruthSchemaV1, Truth: "create"},
	} {
		if err := decision.ValidateFor(input); err == nil {
			t.Fatalf("invalid truth accepted: %+v", decision)
		}
	}
}

func TestKnownArtifactTruthRejectsPhysicalArtifactQuote(t *testing.T) {
	t.Parallel()
	for _, quote := range []string{
		"./obsolete.go must no longer exist.",
		"Remove internal/legacy/adapter.",
	} {
		if _, err := NewKnownArtifactTruthJob(KnownArtifactTruthInput{RequirementQuote: quote}); err == nil {
			t.Fatalf("physical quote accepted: %q", quote)
		}
	}
}
