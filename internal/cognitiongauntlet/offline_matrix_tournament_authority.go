package cognitiongauntlet

import "fmt"

const matrixTournamentSelectionPolicyV1 = "qualified-success.valid-terminal.causal.clean-desk.context.calls.tools.reacquisition.incumbent-tie.v1"

type OfflineMatrixRoundKind string

const (
	MatrixRoundNormative OfflineMatrixRoundKind = "normative_architecture_selection"
	MatrixRoundBenchmark OfflineMatrixRoundKind = "benchmark_only_comparison"
)

type OfflineMatrixRound struct {
	ID         string                 `json:"id"`
	Kind       OfflineMatrixRoundKind `json:"kind"`
	Challenger Variant                `json:"challenger"`
}

func (round OfflineMatrixRound) Validate() error {
	if err := requireExact(round.ID, "offline matrix tournament round ID", 128); err != nil {
		return err
	}
	if !validVariant(round.Challenger) ||
		(round.Kind != MatrixRoundNormative && round.Kind != MatrixRoundBenchmark) {
		return fmt.Errorf("offline matrix tournament round is invalid")
	}
	return nil
}

func offlineMatrixVariantOrder() []Variant {
	return []Variant{
		VariantRawObservation, VariantFullTranscript, VariantTranscriptCompacted,
		VariantTaskLedger, VariantLedgerWorkingSet, VariantLedgerProjection,
		VariantFullCognition, VariantRawShell, VariantOracleEvidence,
	}
}

func offlineMatrixTournamentRounds() []OfflineMatrixRound {
	return []OfflineMatrixRound{
		{ID: "observation-vs-transcript", Kind: MatrixRoundNormative, Challenger: VariantFullTranscript},
		{ID: "winner-vs-task-ledger", Kind: MatrixRoundNormative, Challenger: VariantTaskLedger},
		{ID: "winner-vs-working-set", Kind: MatrixRoundNormative, Challenger: VariantLedgerWorkingSet},
		{ID: "winner-vs-context-projection", Kind: MatrixRoundNormative, Challenger: VariantLedgerProjection},
		{ID: "winner-vs-full-cognition", Kind: MatrixRoundNormative, Challenger: VariantFullCognition},
		{ID: "final-architecture-vs-raw-shell", Kind: MatrixRoundBenchmark, Challenger: VariantRawShell},
	}
}
