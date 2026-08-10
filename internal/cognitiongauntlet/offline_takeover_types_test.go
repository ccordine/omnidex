package cognitiongauntlet

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
)

func TestOfflineTakeoverConfigRequiresAnInteriorDurableBoundary(t *testing.T) {
	t.Parallel()
	promotion := offlineIdentityConfig(
		t, strings.Repeat("a", 40), strings.Repeat("b", 64), strings.Repeat("c", 64),
	)
	config := OfflineTakeoverConfig{
		Schema: OfflineTakeoverConfigSchemaV1, Promotion: promotion, AfterSuccessfulActions: 1,
	}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	config.AfterSuccessfulActions = uint32(promotion.Spec.Budget.EnvironmentActions)
	if err := config.Validate(); err == nil {
		t.Fatal("takeover accepted a boundary at terminal budget exhaustion")
	}
}

func TestOfflineTakeoverReceiptRejectsAuthorityReuse(t *testing.T) {
	t.Parallel()
	before := testSemanticPreCallCheckpoint(1, "worker-before", "projection-before", "snapshot-before")
	after := testSemanticPreCallCheckpoint(2, "worker-after", "projection-after", "snapshot-after")
	proof, err := NewTakeoverContinuityProof(before, after)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	receipt := OfflineTakeoverReceipt{
		Schema:                   OfflineTakeoverReceiptSchemaV1,
		PublicRunAuthoritySHA256: strings.Repeat("a", 64), EpisodeSealSHA256: strings.Repeat("b", 64),
		EvaluationOracleSHA256: strings.Repeat("c", 64), ExecutableSHA256: strings.Repeat("d", 64),
		EvaluationArtifactSHA256: strings.Repeat("5", 64),
		SourceSHA256:             strings.Repeat("e", 64), MigrationsSHA256: strings.Repeat("1", 64),
		RuntimeVersion: "runtime.v1", OmnidexCommit: strings.Repeat("f", 40),
		Original: model.StepAttemptAuthority{
			JobID: 71, Generation: 4, StepID: 9, Attempt: 1, WorkerID: "worker-before",
		},
		Replacement: model.StepAttemptAuthority{
			JobID: 71, Generation: 4, StepID: 9, Attempt: 2, WorkerID: "worker-after",
		},
		GeneratorPID: 10, OriginalPID: 11, ReplacementPID: 12, EvaluatorPID: 13,
		Host: OfflineHostReceipt{
			Schema: offlineHostReceiptSchemaV1, PID: 14, Role: "restricted-host",
			Scenario:     cognition.ScenarioRef{ID: "scenario", SHA256: strings.Repeat("2", 64)},
			ConfigSHA256: strings.Repeat("3", 64), ReadySHA256: strings.Repeat("4", 64),
			StartedAt: now.Add(-500 * time.Millisecond), ExitedAt: now.Add(1500 * time.Millisecond),
		},
		GeneratorExitedAt: now.Add(-time.Second), OriginalKilledAt: now,
		ReplacementExitedAt: now.Add(time.Second),
		EvaluatorStartedAt:  now.Add(2 * time.Second), CompletedAt: now.Add(3 * time.Second),
		Continuity: proof,
	}
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	receipt.Replacement.WorkerID = receipt.Original.WorkerID
	if err := receipt.Validate(); err == nil {
		t.Fatal("takeover receipt accepted reused worker authority")
	}
}
