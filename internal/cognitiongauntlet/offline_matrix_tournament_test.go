package cognitiongauntlet

import "testing"

func TestOfflineMatrixTournamentAppliesPreregisteredRoundsAfterSealedRuns(t *testing.T) {
	registration := matrixRegistrationForGate(t, CompetenceSuccessSuperiority, 6)
	runs := matrixRunsForGate(registration)
	successes := map[Variant]int{
		VariantRawObservation: 2, VariantFullTranscript: 3,
		VariantTranscriptCompacted: 6, VariantTaskLedger: 3,
		VariantLedgerWorkingSet: 2, VariantLedgerProjection: 4,
		VariantFullCognition: 5, VariantRawShell: 6, VariantOracleEvidence: 6,
	}
	seen := map[Variant]int{}
	for index := range runs {
		variant := runs[index].Variant
		qualified := seen[variant] < successes[variant]
		seen[variant]++
		runs[index].GoalSuccess = qualified
		runs[index].ValidTerminalState = qualified
		runs[index].CompetenceQualified = qualified
		runs[index].ModelVisibleBytes = tournamentTestBytes(variant)
	}
	tournament, err := deriveOfflineMatrixTournament(registration, runs)
	if err != nil {
		t.Fatal(err)
	}
	if tournament.StrongestBaseline != VariantLedgerProjection ||
		tournament.FinalArchitecture != VariantFullCognition || len(tournament.Rounds) != 6 {
		t.Fatalf("tournament=%+v", tournament)
	}
	winners := []Variant{
		VariantFullTranscript, VariantTaskLedger, VariantTaskLedger,
		VariantLedgerProjection, VariantFullCognition, VariantRawShell,
	}
	for index, winner := range winners {
		if tournament.Rounds[index].Winner != winner {
			t.Fatalf("round %d winner=%s want=%s", index+1, tournament.Rounds[index].Winner, winner)
		}
	}
}

func TestOfflineMatrixTournamentKeepsIncumbentOnAnExactTie(t *testing.T) {
	registration := matrixRegistrationForGate(t, CompetenceSuccessSuperiority, 1)
	runs := matrixRunsForGate(registration)
	for index := range runs {
		runs[index].ModelVisibleBytes = 500
		runs[index].ModelCalls = 2
		runs[index].ToolOperations = 5
		runs[index].Reacquisitions = 1
	}
	tournament, err := deriveOfflineMatrixTournament(registration, runs)
	if err != nil {
		t.Fatal(err)
	}
	if tournament.StrongestBaseline != VariantRawObservation ||
		tournament.FinalArchitecture != VariantRawObservation {
		t.Fatalf("exact ties did not retain the preregistered incumbent: %+v", tournament)
	}
	for _, round := range tournament.Rounds {
		if round.DecidedBy != "incumbent_tie" || round.Winner != VariantRawObservation {
			t.Fatalf("tie round=%+v", round)
		}
	}
}

func TestOfflineMatrixOracleBoundsRequireEveryVariantToAgree(t *testing.T) {
	registration := matrixRegistrationForGate(t, CompetenceSuccessSuperiority, 1)
	runs := matrixRunsForGate(registration)
	bounds, err := deriveOfflineMatrixOracleBounds(registration, runs)
	if err != nil || len(bounds) != 1 || bounds[0].OracleSHA256 != runs[0].OracleSHA256 {
		t.Fatalf("bounds=%+v error=%v", bounds, err)
	}
	runs[len(runs)-1].OracleReferenceDecisionCost++
	if _, err := deriveOfflineMatrixOracleBounds(registration, runs); err == nil {
		t.Fatal("private evaluator accepted variant disagreement on the deterministic oracle bound")
	}
}

func tournamentTestBytes(variant Variant) int64 {
	switch variant {
	case VariantTaskLedger:
		return 900
	case VariantTranscriptCompacted, VariantOracleEvidence:
		return 1
	default:
		return 1_000
	}
}
