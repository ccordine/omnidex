package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/roleplay"
)

func TestObjectiveCompletionMetadataRequiresExactWebFreshnessAuthority(t *testing.T) {
	record := exactObjectiveCitationRecord("web_document")
	record.Metadata["objective_kind"] = "external_answer"
	record.Metadata["paragraph_indexes"] = []int{0, 2}
	record.Metadata["source_observed_at"] = "2026-01-02T03:04:05.123456Z"
	record.Metadata["source_truncated"] = true
	record.RequirementAuthorityBindings = []string{
		"requirement-test#paragraph-1", "requirement-test#paragraph-3",
	}
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

func TestObjectiveCompletionMetadataKeepsRoleplayResearchInRealWorldAuthority(t *testing.T) {
	record := exactObjectiveCitationRecord("web_document")
	record.Metadata["objective_kind"] = "external_answer"
	record.Metadata["paragraph_indexes"] = []int{0}
	record.Metadata["source_observed_at"] = "2026-01-02T03:04:05.123456Z"
	record.Metadata["source_truncated"] = false
	record.Metadata["authority_namespace"] = string(roleplay.AuthorityRealWorld)
	record.Metadata["roleplay_research_preparation_id"] = "rpt_0123456789abcdef0123456789abcdef"
	record.Metadata["roleplay_research_world_id"] = "rpw_0123456789abcdef0123456789abcdef"
	record.Metadata["roleplay_research_character_id"] = "rpc_0123456789abcdef0123456789abcdef"
	record.Metadata["roleplay_research_question_sha256"] = strings.Repeat("c", 64)
	record.Metadata["roleplay_research_capability_grant_id"] = "rpg_0123456789abcdef0123456789abcdef"
	record.RequirementAuthorityBindings = []string{"requirement-test#paragraph-1"}
	if _, err := normalizeObjectiveCompletionEvidence(record, 7, 9); err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func(*evidence.Record){
		"missing exact authority": func(value *evidence.Record) {
			delete(value.Metadata, "roleplay_research_character_id")
		},
		"fictional namespace": func(value *evidence.Record) {
			value.Metadata["authority_namespace"] = string(roleplay.AuthorityFictionalCanon)
		},
		"non-web source": func(value *evidence.Record) {
			value.SourceType = "repository_symbol"
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := cloneObjectiveCitationRecord(record)
			mutate(&changed)
			if _, err := normalizeObjectiveCompletionEvidence(changed, 7, 9); err == nil {
				t.Fatal("invalid roleplay research authority was accepted")
			}
		})
	}
}

func TestObjectiveCompletionMetadataRequiresExactDatabaseFreshnessAuthority(t *testing.T) {
	record := exactObjectiveCitationRecord("postgres_query")
	record.Metadata["objective_kind"] = "database_read"
	record.Metadata["source_acquired_at"] = "2026-01-02T03:04:05.123456Z"
	if _, err := normalizeObjectiveCompletionEvidence(record, 7, 9); err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func(*evidence.Record){
		"missing acquisition": func(value *evidence.Record) {
			delete(value.Metadata, "source_acquired_at")
		},
		"noncanonical UTC": func(value *evidence.Record) {
			value.Metadata["source_acquired_at"] = "2026-01-02T03:04:05.123456+00:00"
		},
		"wrong kind": func(value *evidence.Record) {
			value.Metadata["objective_kind"] = "repository_read"
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := cloneObjectiveCitationRecord(record)
			mutate(&changed)
			if _, err := normalizeObjectiveCompletionEvidence(changed, 7, 9); err == nil {
				t.Fatal("invalid database metadata was accepted")
			}
		})
	}

	repository := exactObjectiveCitationRecord("repository_symbol")
	repository.Metadata["source_acquired_at"] = record.Metadata["source_acquired_at"]
	if _, err := normalizeObjectiveCompletionEvidence(repository, 7, 9); err == nil {
		t.Fatal("database acquisition authority was accepted on repository evidence")
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

func TestObjectiveCompletionRejectsLegacyRequirementAuthorityBindingKey(t *testing.T) {
	record := exactObjectiveCitationRecord("repository_symbol")
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	legacyKey := "supports_" + "claims"
	payload[legacyKey] = payload["requirement_authority_bindings"]
	delete(payload, "requirement_authority_bindings")
	raw, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var decoded evidence.Record
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.RequirementAuthorityBindings) != 0 {
		t.Fatal("legacy requirement binding key populated current authority")
	}
	if _, err := normalizeObjectiveCompletionEvidence(decoded, 7, 9); err == nil ||
		!strings.Contains(err.Error(), "requires between 1 and 4 requirement authority bindings") {
		t.Fatalf("legacy requirement binding error=%v", err)
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
		RequirementAuthorityBindings: []string{"requirement-test"},
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
	record.RequirementAuthorityBindings = append(
		[]string(nil), record.RequirementAuthorityBindings...,
	)
	record.Metadata = make(map[string]any, len(record.Metadata))
	for key, value := range record.Metadata {
		record.Metadata[key] = value
	}
	return record
}
