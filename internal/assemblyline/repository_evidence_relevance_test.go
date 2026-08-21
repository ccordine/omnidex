package assemblyline

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestRepositoryEvidenceRelevanceReturnsOnlySelectedIDsOrExplicitEmptyIDs(t *testing.T) {
	t.Parallel()
	input := repositoryEvidenceRelevanceFixture()
	job, err := NewRepositoryEvidenceRelevanceJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkRepositoryEvidenceRelevance {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, input.ExactRequirement) || !strings.Contains(prompt, input.Candidates[0].Text) {
		t.Fatalf("prompt lost exact bounded projection: %q", prompt)
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("schema is not closed: %#v", schema)
	}

	selected := RepositoryEvidenceRelevanceDecision{
		Schema:      RepositoryEvidenceRelevanceSchemaV1,
		EvidenceIDs: []string{"R02"},
	}
	if err := selected.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	none := RepositoryEvidenceRelevanceDecision{
		Schema:      RepositoryEvidenceRelevanceSchemaV1,
		EvidenceIDs: []string{},
	}
	if err := none.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	assertExactJSONFields(t, reflect.TypeOf(RepositoryEvidenceRelevanceDecision{}), []string{"schema", "evidence_ids"})
	properties := schema["properties"].(map[string]any)
	if len(properties) != 2 || properties["outcome"] != nil {
		t.Fatalf("repository relevance schema exposes a redundant control outcome: %#v", schema)
	}
	evidenceIDs := properties["evidence_ids"].(map[string]any)
	if evidenceIDs["minItems"] != 0 || evidenceIDs["maxItems"] != input.MaxSelections {
		t.Fatalf("repository evidence ID bounds=%#v", evidenceIDs)
	}
}

func TestRepositoryEvidenceRelevanceRejectsAmbiguousOrUnboundSelections(t *testing.T) {
	t.Parallel()
	input := repositoryEvidenceRelevanceFixture()
	tests := map[string]RepositoryEvidenceRelevanceDecision{
		"nil IDs":      {Schema: RepositoryEvidenceRelevanceSchemaV1},
		"unknown ID":   {Schema: RepositoryEvidenceRelevanceSchemaV1, EvidenceIDs: []string{"R99"}},
		"duplicate ID": {Schema: RepositoryEvidenceRelevanceSchemaV1, EvidenceIDs: []string{"R01", "R01"}},
		"too many":     {Schema: RepositoryEvidenceRelevanceSchemaV1, EvidenceIDs: []string{"R01", "R02"}},
	}
	for name, decision := range tests {
		decision := decision
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := decision.ValidateFor(input); err == nil {
				t.Fatalf("accepted %#v", decision)
			}
		})
	}
}

func TestRepositoryEvidenceRelevanceDecodeRejectsExtraOrDuplicateState(t *testing.T) {
	t.Parallel()
	input := repositoryEvidenceRelevanceFixture()
	valid := fmt.Sprintf(
		`{"schema":%q,"evidence_ids":["R01"]}`,
		RepositoryEvidenceRelevanceSchemaV1,
	)
	if _, err := DecodeRepositoryEvidenceRelevanceDecision(input, valid); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		strings.TrimSuffix(valid, "}") + `,"extra":true}`,
		fmt.Sprintf(`{"schema":%q,"schema":%q,"evidence_ids":[]}`,
			RepositoryEvidenceRelevanceSchemaV1, RepositoryEvidenceRelevanceSchemaV1),
		fmt.Sprintf(`{"schema":%q,"outcome":"none","evidence_ids":[]}`,
			RepositoryEvidenceRelevanceSchemaV1),
	} {
		if _, err := DecodeRepositoryEvidenceRelevanceDecision(input, raw); err == nil {
			t.Fatalf("malformed decision accepted: %s", raw)
		}
	}
}

func TestRepositoryEvidenceRelevanceRejectsUnboundedProjectionBeforeRendering(t *testing.T) {
	t.Parallel()
	input := repositoryEvidenceRelevanceFixture()
	input.ExactRequirement = strings.Repeat("q", maxGroundedRequirementBytes+1)
	if _, err := NewRepositoryEvidenceRelevanceJob(input); err == nil {
		t.Fatal("oversized exact requirement accepted")
	}
	input = repositoryEvidenceRelevanceFixture()
	input.Candidates[0].Text = strings.Repeat("e", maxGroundedEvidenceTextBytes+1)
	if _, err := NewRepositoryEvidenceRelevanceJob(input); err == nil {
		t.Fatal("oversized evidence candidate accepted")
	}
	input = repositoryEvidenceRelevanceFixture()
	for index := 0; index < maxRepositoryRelevanceCandidates; index++ {
		input.Candidates = append(input.Candidates, RepositoryEvidenceCandidate{
			EvidenceID: fmt.Sprintf("E%d", index), Text: "bounded unique evidence " + fmt.Sprint(index),
		})
	}
	if _, err := NewRepositoryEvidenceRelevanceJob(input); err == nil {
		t.Fatal("unbounded candidate count accepted")
	}
}

func repositoryEvidenceRelevanceFixture() RepositoryEvidenceRelevanceInput {
	return RepositoryEvidenceRelevanceInput{
		ExactRequirement: "Explain which component owns dispatch timing.",
		Candidates: []RepositoryEvidenceCandidate{
			{EvidenceID: "R01", Text: "func ScheduleDispatch() starts the delivery timer."},
			{EvidenceID: "R02", Text: "func RecordDelivery() stores the final delivery receipt."},
		},
		MaxSelections: 1,
	}
}
