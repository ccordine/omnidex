package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
)

func TestObjectiveCitationMetadataUsesActualExcerptAndOwnership(t *testing.T) {
	t.Parallel()
	for _, sourceType := range []string{"web_document", "postgres_query"} {
		t.Run(sourceType, func(t *testing.T) {
			record := objectiveCitationMetadataFixture(sourceType)
			payload, err := normalizeObjectiveCompletionEvidence(record, record.JobID, record.StepID)
			if err != nil {
				t.Fatal(err)
			}
			var decoded evidence.Record
			if err := json.Unmarshal(payload, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.Excerpt != record.Excerpt || decoded.SourceRef != record.SourceRef ||
				decoded.JobID != record.JobID || decoded.StepID != record.StepID {
				t.Fatalf("actual evidence changed: %#v", decoded)
			}
			if _, err := normalizeObjectiveCompletionEvidence(record, record.JobID+1, record.StepID); err == nil {
				t.Fatal("citation from another job was accepted")
			}
			for _, key := range []string{"instruction_sha256", "projection_sha256", "source_sha256", "roleplay_research_question_sha256"} {
				invalid := objectiveCitationMetadataFixture(sourceType)
				invalid.Metadata[key] = strings.Repeat("b", 64)
				if _, err := normalizeObjectiveCompletionEvidence(invalid, invalid.JobID, invalid.StepID); err == nil {
					t.Errorf("retired %q metadata was accepted", key)
				}
			}
			record.Excerpt = "invalid\x00text"
			if _, err := normalizeObjectiveCompletionEvidence(record, record.JobID, record.StepID); err == nil {
				t.Fatal("invalid actual excerpt was accepted")
			}
		})
	}
}

func objectiveCitationMetadataFixture(sourceType string) evidence.Record {
	requirement := "objective-17-2-requirement"
	record := evidence.Record{
		JobID: 17, StepID: 18, Kind: evidence.KindObjectiveCitation,
		SourceType: sourceType, SourceRef: "fixture-source", Excerpt: "The exact observed value.",
		Summary: "Observed source value.", Confidence: 1,
		RequirementAuthorityBindings: []string{requirement},
		Metadata: map[string]any{
			"capsule_id": "E1", "objective_id": "objective-17-2",
			"requirement_id": requirement,
		},
	}
	if sourceType == "web_document" {
		record.Metadata["web_evidence_id"] = "41"
		record.Metadata["web_evidence_index"] = "0"
		record.RequirementAuthorityBindings[0] += "#paragraph-1"
		record.Metadata["objective_kind"] = "external_answer"
		record.Metadata["paragraph_indexes"] = []int{0}
		record.Metadata["source_observed_at"] = "2026-09-08T12:00:00Z"
		record.Metadata["source_truncated"] = false
	} else {
		record.Metadata["objective_kind"] = "database_read"
		record.Metadata["source_acquired_at"] = "2026-09-08T12:00:00Z"
		record.Metadata["database_evidence_id"] = "41"
		record.Metadata["database_row_start"] = "0"
		record.Metadata["database_row_end"] = "1"
	}
	return record
}
