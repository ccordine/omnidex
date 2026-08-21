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
		`{"schema":"omnidex.database-evidence-gap.v1","requirement_id":"requirement-1","missing_information":"The next-visit miss rate for the control cohort."}`)
	if err != nil || missing.Missing() == "" {
		t.Fatalf("missing=%+v err=%v", missing, err)
	}
	complete, err := DecodeDatabaseEvidenceGapDecision(input,
		`{"schema":"omnidex.database-evidence-gap.v1","requirement_id":"requirement-1","missing_information":""}`)
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
		"control": `{"schema":"omnidex.database-evidence-gap.v1","requirement_id":"requirement-1","missing_information":"","continue":false}`,
		"null":    `{"schema":"omnidex.database-evidence-gap.v1","requirement_id":"requirement-1","missing_information":null}`,
		"padded":  `{"schema":"omnidex.database-evidence-gap.v1","requirement_id":"requirement-1","missing_information":" more "}`,
		"oversized": `{"schema":"omnidex.database-evidence-gap.v1","requirement_id":"requirement-1","missing_information":"` +
			strings.Repeat("x", maxDatabaseEvidenceGapBytes+1) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeDatabaseEvidenceGapDecision(input, raw); err == nil {
				t.Fatalf("accepted invalid decision: %s", raw)
			}
		})
	}
}

func TestDatabaseEvidenceGapResponseSchemaLeavesByteCeilingToCode(t *testing.T) {
	input := DatabaseEvidenceGapInput{
		RequirementID: "requirement-1", ExactRequirement: "How many records exist?",
		Evidence: []GroundedEvidenceCapsule{{ID: "E1", Text: "Record count: 4."}},
	}
	schema, err := DatabaseEvidenceGapResponseSchema(input)
	if err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	missing := properties["missing_information"].(map[string]any)
	if _, finiteGrammarBound := missing["maxLength"]; finiteGrammarBound {
		t.Fatalf("database evidence-gap schema encodes the code-owned byte ceiling: %#v", missing)
	}
}
