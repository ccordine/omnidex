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
		RequirementID: "requirement-1", ExactRequirement: "How many records exist?",
		Evidence: []GroundedEvidenceCapsule{{ID: "E1", Text: "Record count: 4."}},
	}
	job, err := NewDatabaseEvidenceGapJob(input)
	if err != nil {
		t.Fatal(err)
	}
	_, err = RenderPortableJob(job)
	if err != nil {
		t.Fatalf("render database evidence gap: %v", err)
	}
}
