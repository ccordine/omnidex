package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func TestStationCallEvidenceIsDerivedFromDurableCallAuthority(t *testing.T) {
	t.Parallel()
	authority := model.StepAttemptAuthority{
		JobID: 3, Generation: 2, StepID: 7, Attempt: 1, WorkerID: "worker-a",
	}
	gap := stationCallTestGap(t, authority)
	prepared := stationCallTestPrepared(t, gap)
	opening, err := validateStationCallOpening(StationCallOpenRecord{
		Authority: authority, Gap: gap,
		Discovery: stationCallTestDiscovery(t, gap, prepared), Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	opening.ID = 23
	result := stationCallSuccess(t, prepared, opening)
	generation, err := validateStationCallReceipt(StationCallReceiptRecord{
		Authority: authority, OpeningID: opening.ID, GapID: opening.GapID, Result: result,
	}, opening)
	if err != nil {
		t.Fatal(err)
	}
	receipt := StationCallReceipt{
		ID: 29, OpeningID: opening.ID, JobID: authority.JobID, Generation: authority.Generation,
		StepID: authority.StepID, StepAttempt: authority.Attempt, WorkerID: authority.WorkerID,
		GapID: opening.GapID, Status: "succeeded", GenerationJSON: generation,
	}
	evidence, err := receipt.LLMCallEvidenceRecord(authority, gap, opening, "requested", 1, 17)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.StationCallOpeningID != opening.ID || evidence.SystemPrompt != gap.Prompt ||
		evidence.Response != result.Content || evidence.WorkID != opening.GapID ||
		evidence.ContextTokens != opening.ContextTokens {
		t.Fatalf("derived evidence=%+v", evidence)
	}
}

func TestDurableStationGenerationRejectsThinkingAuthority(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"thinking", "Thinking"} {
		field := field
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			raw := []byte(`{"schema":"` + llm.PreparedGenerationSchemaV1 +
				`","` + field + `":"forbidden trace"}`)
			if _, err := decodeDurableStationGeneration(raw); err == nil ||
				!strings.Contains(err.Error(), field) {
				t.Fatalf("durable generation accepted %q authority: %v", field, err)
			}
		})
	}
}
