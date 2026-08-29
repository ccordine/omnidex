package assemblyline

import (
	"strings"
	"testing"
)

func TestDatabaseEvidenceGapReturnsOnlyOneMissingSemanticLeaf(t *testing.T) {
	input := DatabaseEvidenceGapInput{
		RequirementID: "requirement-1", ExactRequirement: "Are repeat cancellers more likely to miss their next visit?",
		Evidence: []GroundedEvidenceCapsule{{ID: "E1", Text: "Repeat-canceller cohort: 40 patients; 10 missed the next visit."}},
	}
	job, err := NewDatabaseEvidenceGapJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkDatabaseEvidenceGap {
		t.Fatalf("kind=%q", job.Kind)
	}
	missing, err := DecodeDatabaseEvidenceGapDecision(input,
		"The next-visit miss rate for the control cohort.")
	if err != nil || missing.Missing() == "" {
		t.Fatalf("missing=%+v err=%v", missing, err)
	}
	complete, err := DecodeDatabaseEvidenceGapDecision(input,
		DatabaseEvidenceGapNone)
	if err != nil || complete.Missing() != "" {
		t.Fatalf("complete=%+v err=%v", complete, err)
	}
}

func TestDatabaseEvidenceGapKeepsKnownArtifactPathsOutOfPrompt(t *testing.T) {
	t.Parallel()
	input := DatabaseEvidenceGapInput{
		RequirementID: "requirement-1", ExactRequirement: "Count ARTIFACT_1 records.",
		Evidence:           []GroundedEvidenceCapsule{{ID: "E1", Text: "Record count: 4."}},
		KnownArtifactPaths: []string{"internal/private/records.sql"},
	}
	prompt, err := BuildDatabaseEvidenceGapPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, "internal/private/records.sql") || !strings.Contains(prompt, "ARTIFACT_1") {
		t.Fatalf("database gap prompt crossed artifact boundary: %s", prompt)
	}
	input.ExactRequirement = "Count records.sql records."
	if _, err := BuildDatabaseEvidenceGapPrompt(input); err == nil {
		t.Fatal("current-tree basename reached database gap prompt")
	}
}

func TestDatabaseEvidenceGapRejectsControlAndImplicitFields(t *testing.T) {
	input := DatabaseEvidenceGapInput{
		RequirementID: "requirement-1", ExactRequirement: "How many records exist?",
		Evidence: []GroundedEvidenceCapsule{{ID: "E1", Text: "Record count: 4."}},
	}
	for name, raw := range map[string]string{
		"control JSON": `{"missing_information":"","continue":false}`,
		"null JSON":    `null`,
		"quoted":       `"more"`,
		"oversized":    strings.Repeat("x", maxDatabaseEvidenceGapBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeDatabaseEvidenceGapDecision(input, raw); err == nil {
				t.Fatalf("accepted invalid decision: %s", raw)
			}
		})
	}
}

func TestDatabaseEvidenceGapPortableJobHasNoResponseSchema(t *testing.T) {
	input := DatabaseEvidenceGapInput{
		RequirementID: "requirement_hidden_9173", ExactRequirement: "How many records exist?",
		Evidence: []GroundedEvidenceCapsule{{ID: "evidence_hidden_2846", Text: "Record count: 4."}},
	}
	job, err := NewDatabaseEvidenceGapJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatalf("render database evidence gap: %v", err)
	}
	for _, visible := range []string{input.ExactRequirement, input.Evidence[0].Text} {
		if !strings.Contains(prompt, visible) {
			t.Fatalf("database evidence gap prompt omitted semantic authority %q: %s", visible, prompt)
		}
	}
	for _, hidden := range []string{input.RequirementID, input.Evidence[0].ID} {
		if strings.Contains(prompt, hidden) {
			t.Fatalf("database evidence gap prompt exposed code-owned ID %q: %s", hidden, prompt)
		}
		if !strings.Contains(string(job.Payload), hidden) {
			t.Fatalf("database evidence gap payload lost code-owned binding %q: %s", hidden, job.Payload)
		}
	}
}
