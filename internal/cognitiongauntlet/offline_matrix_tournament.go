package cognitiongauntlet

import (
	"fmt"
	"reflect"
)

type OfflineMatrixVariantAggregate struct {
	Variant                Variant `json:"variant"`
	Tasks                  int     `json:"tasks"`
	CompetenceQualified    int     `json:"competence_qualified"`
	ValidTerminals         int     `json:"valid_terminals"`
	CausalAdmissions       int     `json:"causal_admissions"`
	CleanDeskQualified     int     `json:"clean_desk_qualified"`
	TotalModelVisibleBytes int64   `json:"total_model_visible_bytes"`
	TotalModelCalls        int64   `json:"total_model_calls"`
	TotalToolOperations    int64   `json:"total_tool_operations"`
	TotalReacquisitions    int64   `json:"total_reacquisitions"`
}

type OfflineMatrixRoundResult struct {
	Round      OfflineMatrixRound            `json:"round"`
	Incumbent  OfflineMatrixVariantAggregate `json:"incumbent"`
	Challenger OfflineMatrixVariantAggregate `json:"challenger"`
	Winner     Variant                       `json:"winner"`
	DecidedBy  string                        `json:"decided_by"`
}

type OfflineMatrixTournament struct {
	SelectionPolicy   string                     `json:"selection_policy"`
	Seed              Variant                    `json:"seed"`
	StrongestBaseline Variant                    `json:"strongest_baseline"`
	FinalArchitecture Variant                    `json:"final_architecture"`
	Rounds            []OfflineMatrixRoundResult `json:"rounds"`
}

func deriveOfflineMatrixTournament(
	registration OfflineMatrixPreregistration,
	runs []OfflineMatrixRunReceipt,
) (OfflineMatrixTournament, error) {
	indexed, err := indexOfflineMatrixRuns(registration, runs)
	if err != nil {
		return OfflineMatrixTournament{}, err
	}
	tournament := OfflineMatrixTournament{
		SelectionPolicy: registration.TournamentSelectionPolicy,
		Seed:            registration.TournamentSeed,
		Rounds:          make([]OfflineMatrixRoundResult, 0, len(registration.TournamentRounds)),
	}
	incumbent := registration.TournamentSeed
	for _, round := range registration.TournamentRounds {
		left, err := aggregateOfflineMatrixVariant(registration, indexed, incumbent)
		if err != nil {
			return OfflineMatrixTournament{}, err
		}
		right, err := aggregateOfflineMatrixVariant(registration, indexed, round.Challenger)
		if err != nil {
			return OfflineMatrixTournament{}, err
		}
		winner, decidedBy := selectOfflineMatrixWinner(left, right)
		result := OfflineMatrixRoundResult{
			Round: round, Incumbent: left, Challenger: right,
			Winner: winner, DecidedBy: decidedBy,
		}
		tournament.Rounds = append(tournament.Rounds, result)
		if round.Kind == MatrixRoundBenchmark {
			continue
		}
		if round.Challenger == VariantFullCognition {
			tournament.StrongestBaseline = incumbent
		}
		incumbent = winner
		if round.Challenger == VariantFullCognition {
			tournament.FinalArchitecture = incumbent
		}
	}
	if tournament.StrongestBaseline == "" || tournament.FinalArchitecture == "" {
		return OfflineMatrixTournament{}, fmt.Errorf("offline matrix tournament lacks the full cognition round")
	}
	return tournament, nil
}

func indexOfflineMatrixRuns(
	registration OfflineMatrixPreregistration,
	runs []OfflineMatrixRunReceipt,
) (map[string]OfflineMatrixRunReceipt, error) {
	if len(runs) != registration.RunCount {
		return nil, fmt.Errorf("offline matrix run count differs from preregistration")
	}
	indexed := make(map[string]OfflineMatrixRunReceipt, len(runs))
	for _, run := range runs {
		key := run.Case.ID + "\x00" + string(run.Variant)
		if _, duplicate := indexed[key]; duplicate {
			return nil, fmt.Errorf("offline matrix contains duplicate run %q", key)
		}
		indexed[key] = run
	}
	return indexed, nil
}

func aggregateOfflineMatrixVariant(
	registration OfflineMatrixPreregistration,
	indexed map[string]OfflineMatrixRunReceipt,
	variant Variant,
) (OfflineMatrixVariantAggregate, error) {
	value := OfflineMatrixVariantAggregate{Variant: variant, Tasks: len(registration.Cases)}
	for _, currentCase := range registration.Cases {
		run, exists := indexed[currentCase.ID+"\x00"+string(variant)]
		if !exists {
			return OfflineMatrixVariantAggregate{}, fmt.Errorf(
				"offline matrix case %q lacks variant %q", currentCase.ID, variant,
			)
		}
		value.CompetenceQualified += boolCount(run.CompetenceQualified)
		value.ValidTerminals += boolCount(run.ValidTerminalState)
		value.CausalAdmissions += boolCount(run.CausalAdmissionComplete)
		value.CleanDeskQualified += boolCount(run.CleanDeskQualified)
		value.TotalModelVisibleBytes += run.ModelVisibleBytes
		value.TotalModelCalls += int64(run.ModelCalls)
		value.TotalToolOperations += int64(run.ToolOperations)
		value.TotalReacquisitions += int64(run.Reacquisitions)
	}
	return value, nil
}

func selectOfflineMatrixWinner(
	incumbent OfflineMatrixVariantAggregate,
	challenger OfflineMatrixVariantAggregate,
) (Variant, string) {
	comparisons := []struct {
		name       string
		incumbent  int64
		challenger int64
		maximize   bool
	}{
		{"competence_qualified", int64(incumbent.CompetenceQualified), int64(challenger.CompetenceQualified), true},
		{"valid_terminals", int64(incumbent.ValidTerminals), int64(challenger.ValidTerminals), true},
		{"causal_admissions", int64(incumbent.CausalAdmissions), int64(challenger.CausalAdmissions), true},
		{"clean_desk_qualified", int64(incumbent.CleanDeskQualified), int64(challenger.CleanDeskQualified), true},
		{"model_visible_bytes", incumbent.TotalModelVisibleBytes, challenger.TotalModelVisibleBytes, false},
		{"model_calls", incumbent.TotalModelCalls, challenger.TotalModelCalls, false},
		{"tool_operations", incumbent.TotalToolOperations, challenger.TotalToolOperations, false},
		{"reacquisitions", incumbent.TotalReacquisitions, challenger.TotalReacquisitions, false},
	}
	for _, comparison := range comparisons {
		if comparison.incumbent == comparison.challenger {
			continue
		}
		challengerWins := comparison.challenger > comparison.incumbent
		if !comparison.maximize {
			challengerWins = comparison.challenger < comparison.incumbent
		}
		if challengerWins {
			return challenger.Variant, comparison.name
		}
		return incumbent.Variant, comparison.name
	}
	return incumbent.Variant, "incumbent_tie"
}

func equalOfflineMatrixTournament(left, right OfflineMatrixTournament) bool {
	return reflect.DeepEqual(left, right)
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
