package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
)

func TestObjectiveCompletionMetadataRequiresExactWebFreshnessAuthority(t *testing.T) {
	record := exactObjectiveCitationRecord("web_document")
	record.Metadata["objective_kind"] = "external_answer"
	record.Metadata["paragraph_indexes"] = []int{0, 2}
	record.Metadata["source_observed_at"] = "2026-01-02T03:04:05.123456Z"
	record.Metadata["source_truncated"] = true
	record.SupportsClaims = []string{"requirement-test#paragraph-1", "requirement-test#paragraph-3"}
	if _, err := normalizeObjectiveCompletionEvidence(record, 7, 9); err != nil {
		t.Fatal(err)
	}

	tests := map[string]func(*evidence.Record){
		"missing observation": func(value *evidence.Record) { delete(value.Metadata, "source_observed_at") },
		"noncanonical UTC": func(value *evidence.Record) {
			value.Metadata["source_observed_at"] = "2026-01-02T03:04:05.123456+00:00"
		},
		"wrong truncation type": func(value *evidence.Record) {
			value.Metadata["source_truncated"] = "true"
		},
		"unordered paragraphs": func(value *evidence.Record) {
			value.Metadata["paragraph_indexes"] = []int{2, 0}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changed := cloneObjectiveCitationRecord(record)
			mutate(&changed)
			if _, err := normalizeObjectiveCompletionEvidence(changed, 7, 9); err == nil {
				t.Fatal("invalid web metadata was accepted")
			}
		})
	}
}

func TestObjectiveCompletionMetadataRejectsWebFieldsOnNonWebEvidence(t *testing.T) {
	record := exactObjectiveCitationRecord("repository_symbol")
	record.Metadata["source_observed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	record.Metadata["source_truncated"] = false
	if _, err := normalizeObjectiveCompletionEvidence(record, 7, 9); err == nil {
		t.Fatalf("non-web freshness error=%v", err)
	}
}

func TestObjectiveCompletionMetadataBoundsBeforeJSONMarshal(t *testing.T) {
	record := exactObjectiveCitationRecord("repository_symbol")
	delete(record.Metadata, "source_sha256")
	record.Metadata["unknown"] = panicObjectiveMetadataMarshaler{}
	if _, err := normalizeObjectiveCompletionEvidence(record, 7, 9); err == nil ||
		!strings.Contains(err.Error(), "unknown metadata field") {
		t.Fatalf("unknown metadata error=%v", err)
	}

	record = exactObjectiveCitationRecord("repository_symbol")
	record.Metadata["objective_id"] = strings.Repeat("x", 129)
	if _, err := normalizeObjectiveCompletionEvidence(record, 7, 9); err == nil ||
		!strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized metadata error=%v", err)
	}
}

type panicObjectiveMetadataMarshaler struct{}

func (panicObjectiveMetadataMarshaler) MarshalJSON() ([]byte, error) {
	panic("unbounded metadata reached JSON marshal")
}

func exactObjectiveCitationRecord(sourceType string) evidence.Record {
	excerpt := "Exact projected evidence."
	projection := sha256.Sum256([]byte(excerpt))
	sourceHash := strings.Repeat("a", 64)
	return evidence.Record{
		JobID: 7, StepID: 9, Kind: evidence.KindObjectiveCitation,
		SourceType: sourceType, SourceRef: "source-ref", Excerpt: excerpt,
		Summary: "Exact objective citation.", Hash: sourceHash, Confidence: 1,
		SupportsClaims: []string{"requirement-test"},
		Metadata: map[string]any{
			"capsule_id": "R01", "instruction_sha256": strings.Repeat("b", 64),
			"objective_id": "objective-test", "objective_kind": "repository_read",
			"requirement_id":    "requirement-test",
			"projection_sha256": hex.EncodeToString(projection[:]),
			"source_sha256":     sourceHash,
		},
	}
}

func cloneObjectiveCitationRecord(record evidence.Record) evidence.Record {
	record.SupportsClaims = append([]string(nil), record.SupportsClaims...)
	record.Metadata = make(map[string]any, len(record.Metadata))
	for key, value := range record.Metadata {
		record.Metadata[key] = value
	}
	return record
}
