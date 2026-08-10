package cognitiongauntlet

import (
	"fmt"
	"math/big"
	"reflect"
	"sort"
)

func deriveOfflineMatrixGate(
	registration OfflineMatrixPreregistration,
	runs []OfflineMatrixRunReceipt,
) (OfflineMatrixGate, error) {
	pairs, tournament, err := normativeMatrixPairs(registration, runs)
	if err != nil {
		return OfflineMatrixGate{}, err
	}
	gate := OfflineMatrixGate{
		Policy: registration.Policy, BaselineVariant: tournament.StrongestBaseline,
		CandidateVariant: VariantFullCognition, Tasks: len(pairs), Reasons: []string{},
	}
	contextReductions := make([]int, 0, len(pairs))
	cleanDeskComplete := true
	for _, pair := range pairs {
		if pair.baseline.CompetenceQualified {
			gate.BaselineSuccesses++
		}
		if pair.candidate.CompetenceQualified {
			gate.CandidateSuccesses++
		}
		if pair.baseline.ValidTerminalState {
			gate.BaselineValidTerminals++
		}
		if pair.candidate.ValidTerminalState {
			gate.CandidateValidTerminals++
		}
		if !pair.baseline.CompetenceQualified && pair.candidate.CompetenceQualified {
			gate.Rescues++
		}
		if pair.baseline.CompetenceQualified && !pair.candidate.CompetenceQualified {
			gate.Regressions++
		}
		gate.ReacquisitionDelta += pair.candidate.Reacquisitions - pair.baseline.Reacquisitions
		gate.ToolOperationDelta += pair.candidate.ToolOperations - pair.baseline.ToolOperations
		if !pair.baseline.CleanDeskAvailable || !pair.candidate.CleanDeskAvailable ||
			pair.baseline.ModelVisibleBytes <= 0 || pair.candidate.MissingCriticalRefs != 0 {
			cleanDeskComplete = false
			continue
		}
		contextReductions = append(contextReductions, signedBasisPoints(
			pair.baseline.ModelVisibleBytes-pair.candidate.ModelVisibleBytes,
			pair.baseline.ModelVisibleBytes,
		))
	}
	gate.DiscordantPairs = gate.Rescues + gate.Regressions
	gate.PairedPValueUpperPPM = exactBinomialTailUpperPPM(gate.DiscordantPairs, gate.Rescues)
	gate.PairedLiftBasisPoints = signedBasisPoints(
		int64(gate.Rescues-gate.Regressions), int64(gate.Tasks),
	)
	gate.SuccessLossBasisPoints = signedBasisPoints(
		int64(gate.BaselineSuccesses-gate.CandidateSuccesses), int64(gate.Tasks),
	)
	if len(contextReductions) == len(pairs) {
		sort.Ints(contextReductions)
		gate.MedianContextReductionPoints = contextReductions[(len(contextReductions)-1)/2]
	}
	if gate.CandidateValidTerminals < gate.BaselineValidTerminals {
		gate.Reasons = append(gate.Reasons, "full cognition reduced valid terminal outcomes")
	}
	if tournament.FinalArchitecture != VariantFullCognition {
		gate.Reasons = append(gate.Reasons, "full cognition did not win the preregistered architecture round")
	}
	switch registration.Policy {
	case CompetenceSuccessSuperiority:
		if gate.PairedLiftBasisPoints < registration.MinimumEffectBasisPoints {
			gate.Reasons = append(gate.Reasons, "paired success lift is below the preregistered effect")
		}
		if gate.Rescues <= gate.Regressions {
			gate.Reasons = append(gate.Reasons, "rescues do not exceed regressions")
		}
		if gate.PairedPValueUpperPPM > registration.AlphaPPM {
			gate.Reasons = append(gate.Reasons, "exact paired test does not meet preregistered alpha")
		}
	case CompetenceEfficiencySuperiority:
		if gate.SuccessLossBasisPoints > registration.MaximumSuccessLossBasisPoints {
			gate.Reasons = append(gate.Reasons, "success loss exceeds the preregistered limit")
		}
		if !cleanDeskComplete {
			gate.Reasons = append(gate.Reasons, "efficiency evidence lacks complete critical clean-desk coverage")
		} else if gate.MedianContextReductionPoints < registration.ContextReductionBasisPoints {
			gate.Reasons = append(gate.Reasons, "median context reduction is below the preregistered threshold")
		}
		if gate.ReacquisitionDelta >= 0 {
			gate.Reasons = append(gate.Reasons, "full cognition did not reduce reacquisitions")
		}
		if gate.ToolOperationDelta >= 0 {
			gate.Reasons = append(gate.Reasons, "full cognition did not reduce tool operations")
		}
	default:
		return OfflineMatrixGate{}, fmt.Errorf("offline matrix competence policy is unregistered")
	}
	gate.Passed = len(gate.Reasons) == 0
	return gate, nil
}

type normativeMatrixPair struct {
	baseline  OfflineMatrixRunReceipt
	candidate OfflineMatrixRunReceipt
}

func normativeMatrixPairs(
	registration OfflineMatrixPreregistration,
	runs []OfflineMatrixRunReceipt,
) ([]normativeMatrixPair, OfflineMatrixTournament, error) {
	tournament, err := deriveOfflineMatrixTournament(registration, runs)
	if err != nil {
		return nil, OfflineMatrixTournament{}, err
	}
	byCoordinate := make(map[string]OfflineMatrixRunReceipt, len(runs))
	for _, run := range runs {
		key := run.Case.ID + "\x00" + string(run.Variant)
		if _, duplicate := byCoordinate[key]; duplicate {
			return nil, OfflineMatrixTournament{}, fmt.Errorf("offline matrix contains duplicate run %q", key)
		}
		byCoordinate[key] = run
	}
	pairs := make([]normativeMatrixPair, 0, registration.SampleCount)
	for _, currentCase := range registration.Cases {
		baseline, baselineOK := byCoordinate[currentCase.ID+"\x00"+string(tournament.StrongestBaseline)]
		candidate, candidateOK := byCoordinate[currentCase.ID+"\x00"+string(VariantFullCognition)]
		if !baselineOK || !candidateOK {
			return nil, OfflineMatrixTournament{}, fmt.Errorf("offline matrix case %q lacks its normative pair", currentCase.ID)
		}
		pairs = append(pairs, normativeMatrixPair{baseline: baseline, candidate: candidate})
	}
	return pairs, tournament, nil
}

func signedBasisPoints(numerator, denominator int64) int {
	if denominator <= 0 {
		return 0
	}
	return int((numerator * 10_000) / denominator)
}

func exactBinomialTailUpperPPM(trials, successes int) int {
	if trials <= 0 || successes < 0 || successes > trials {
		return 1_000_000
	}
	numerator := new(big.Int)
	for value := successes; value <= trials; value++ {
		numerator.Add(numerator, new(big.Int).Binomial(int64(trials), int64(value)))
	}
	denominator := new(big.Int).Lsh(big.NewInt(1), uint(trials))
	scaled := new(big.Int).Mul(numerator, big.NewInt(1_000_000))
	scaled.Add(scaled, new(big.Int).Sub(denominator, big.NewInt(1)))
	scaled.Quo(scaled, denominator)
	if !scaled.IsInt64() || scaled.Int64() > 1_000_000 {
		return 1_000_000
	}
	return int(scaled.Int64())
}

func equalMatrixGate(left, right OfflineMatrixGate) bool {
	return reflect.DeepEqual(left, right)
}
