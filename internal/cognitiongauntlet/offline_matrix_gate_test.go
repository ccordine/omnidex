package cognitiongauntlet

import (
	"strings"
	"testing"
	"time"
)

func TestOfflineMatrixSuccessGateUsesOnlyExactNormativePairs(t *testing.T) {
	registration := matrixRegistrationForGate(t, CompetenceSuccessSuperiority, 6)
	runs := matrixRunsForGate(registration)
	for index := range runs {
		switch runs[index].Variant {
		case VariantRawObservation, VariantFullTranscript, VariantTaskLedger,
			VariantLedgerWorkingSet, VariantLedgerProjection:
			runs[index].GoalSuccess = false
			runs[index].CompetenceQualified = false
		case VariantFullCognition:
			runs[index].GoalSuccess = true
			runs[index].CompetenceQualified = true
		case VariantRawShell, VariantOracleEvidence:
			runs[index].GoalSuccess = true
			runs[index].ModelVisibleBytes = 1
			runs[index].Reacquisitions = 0
			runs[index].ToolOperations = 0
		}
	}
	gate, err := deriveOfflineMatrixGate(registration, runs)
	if err != nil {
		t.Fatal(err)
	}
	if !gate.Passed || gate.Tasks != 6 || gate.Rescues != 6 || gate.Regressions != 0 ||
		gate.PairedPValueUpperPPM != 15_625 {
		t.Fatalf("success gate=%+v", gate)
	}
	changed := cloneMatrixRuns(runs)
	for index := range changed {
		if changed[index].Variant == VariantRawShell || changed[index].Variant == VariantOracleEvidence {
			changed[index].GoalSuccess = false
			changed[index].ValidTerminalState = false
			changed[index].ModelVisibleBytes = 9_000_000
		}
	}
	second, err := deriveOfflineMatrixGate(registration, changed)
	if err != nil || !equalMatrixGate(gate, second) {
		t.Fatalf("contaminated/benchmark ceilings changed normative gate: %+v error=%v", second, err)
	}
}

func TestOfflineMatrixEfficiencyGateNeverRewardsMissingCriticalEvidence(t *testing.T) {
	registration := matrixRegistrationForGate(t, CompetenceEfficiencySuperiority, 2)
	runs := matrixRunsForGate(registration)
	gate, err := deriveOfflineMatrixGate(registration, runs)
	if err != nil {
		t.Fatal(err)
	}
	if !gate.Passed || gate.MedianContextReductionPoints != 5_000 ||
		gate.ReacquisitionDelta >= 0 || gate.ToolOperationDelta >= 0 {
		t.Fatalf("efficiency gate=%+v", gate)
	}
	for index := range runs {
		if runs[index].Variant == VariantFullCognition {
			runs[index].ModelVisibleBytes = 1
			runs[index].MissingCriticalRefs = 1
			runs[index].CleanDeskQualified = false
			runs[index].CompetenceQualified = false
		}
	}
	gate, err = deriveOfflineMatrixGate(registration, runs)
	if err != nil {
		t.Fatal(err)
	}
	if gate.Passed || !containsMatrixReason(gate.Reasons, "critical clean-desk") {
		t.Fatalf("missing critical evidence was rewarded: %+v", gate)
	}
}

func TestOfflineMatrixSuccessGateRejectsGuessedTerminalAsRescue(t *testing.T) {
	registration := matrixRegistrationForGate(t, CompetenceSuccessSuperiority, 6)
	runs := matrixRunsForGate(registration)
	for index := range runs {
		switch runs[index].Variant {
		case VariantRawObservation, VariantFullTranscript, VariantTaskLedger,
			VariantLedgerWorkingSet, VariantLedgerProjection:
			runs[index].GoalSuccess = false
			runs[index].CompetenceQualified = false
		case VariantFullCognition:
			runs[index].GoalSuccess = true
			runs[index].MissingCriticalRefs = 1
			runs[index].CleanDeskQualified = false
			runs[index].CompetenceQualified = false
		}
	}
	gate, err := deriveOfflineMatrixGate(registration, runs)
	if err != nil {
		t.Fatal(err)
	}
	if gate.Passed || gate.Rescues != 0 || gate.CandidateSuccesses != 0 {
		t.Fatalf("guessed terminals were counted as competence: %+v", gate)
	}
}

func TestOfflineMatrixReceiptRequiresEveryCoordinateExactlyOnce(t *testing.T) {
	registration := matrixRegistrationForGate(t, CompetenceEfficiencySuperiority, 1)
	runs := matrixRunsForGate(registration)
	gate, err := deriveOfflineMatrixGate(registration, runs)
	if err != nil {
		t.Fatal(err)
	}
	receipt := matrixReceiptForGate(t, registration, runs, gate)
	if err := receipt.Validate(registration); err != nil {
		t.Fatal(err)
	}
	if !receipt.GateEvidenceQualified || receipt.PromotionEligible {
		t.Fatal("passing diagnostic matrix did not remain non-promotional")
	}
	t.Run("isolated promotion claim", func(t *testing.T) {
		candidate := receipt
		candidate.PromotionEligible = true
		if err := candidate.Validate(registration); err == nil {
			t.Fatal("isolated matrix receipt claimed global promotion")
		}
	})
	for name, changed := range map[string][]OfflineMatrixRunReceipt{
		"missing":   cloneMatrixRuns(runs[:len(runs)-1]),
		"duplicate": append(cloneMatrixRuns(runs[:len(runs)-1]), runs[0]),
		"reordered": append([]OfflineMatrixRunReceipt{runs[1]}, append([]OfflineMatrixRunReceipt{runs[0]}, runs[2:]...)...),
	} {
		t.Run(name, func(t *testing.T) {
			candidate := receipt
			candidate.Runs = changed
			if err := candidate.Validate(registration); err == nil {
				t.Fatalf("matrix receipt accepted %s coordinates", name)
			}
		})
	}
	t.Run("forged tournament", func(t *testing.T) {
		candidate := receipt
		candidate.Tournament.Rounds = cloneMatrixSlice(receipt.Tournament.Rounds)
		candidate.Tournament.Rounds[0].Winner = VariantFullCognition
		if err := candidate.Validate(registration); err == nil {
			t.Fatal("matrix receipt accepted a caller-authored tournament winner")
		}
	})
	t.Run("forged oracle bound", func(t *testing.T) {
		candidate := receipt
		candidate.DeterministicOracleBounds = cloneMatrixSlice(receipt.DeterministicOracleBounds)
		candidate.DeterministicOracleBounds[0].ReferenceDecisionCost++
		if err := candidate.Validate(registration); err == nil {
			t.Fatal("matrix receipt accepted a substituted private oracle bound")
		}
	})
	for name, mutate := range map[string]func(*OfflineMatrixReceipt){
		"last inference": func(value *OfflineMatrixReceipt) {
			value.LastInferenceExitedAt = value.LastInferenceExitedAt.Add(time.Second)
		},
		"first evaluator": func(value *OfflineMatrixReceipt) {
			value.FirstEvaluatorStartedAt = value.FirstEvaluatorStartedAt.Add(time.Second)
		},
		"completion": func(value *OfflineMatrixReceipt) {
			value.CompletedAt = value.CompletedAt.Add(time.Second)
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := receipt
			mutate(&candidate)
			if err := candidate.Validate(registration); err == nil {
				t.Fatal("matrix accepted caller-authored aggregate chronology")
			}
		})
	}
	verified := VerifiedOfflineMatrixReceipt{receipt: receipt}
	copy := verified.Receipt()
	copy.DeterministicOracleBounds[0].ReferenceDecisionCost++
	copy.Tournament.Rounds[0].Winner = VariantFullCognition
	copy.Gate.Reasons = append(copy.Gate.Reasons, "changed")
	if verified.receipt.DeterministicOracleBounds[0].ReferenceDecisionCost ==
		copy.DeterministicOracleBounds[0].ReferenceDecisionCost ||
		verified.receipt.Tournament.Rounds[0].Winner == copy.Tournament.Rounds[0].Winner ||
		len(verified.receipt.Gate.Reasons) == len(copy.Gate.Reasons) {
		t.Fatal("verified matrix receipt exposed mutable nested slices")
	}
}

func matrixRegistrationForGate(
	t *testing.T,
	policy CompetencePolicy,
	seedCount int,
) OfflineMatrixPreregistration {
	t.Helper()
	seeds := make([]uint64, seedCount)
	for index := range seeds {
		seeds[index] = uint64(101 + index)
	}
	registration, err := NewOfflineMatrixPreregistration(OfflineMatrixPlan{
		Policy: policy, Suites: []Suite{SuiteRetrieve}, Seeds: seeds,
		Repetitions: 1, Surface: SurfaceFilesystem,
	}, matrixFixedAuthority(t))
	if err != nil {
		t.Fatal(err)
	}
	return registration
}

func matrixRunsForGate(registration OfflineMatrixPreregistration) []OfflineMatrixRunReceipt {
	runs := make([]OfflineMatrixRunReceipt, 0, registration.RunCount)
	for index, coordinate := range matrixCoordinates(registration) {
		class := MatrixIsolatedProcess
		if coordinate.Variant == VariantRawShell {
			class = MatrixBenchmarkOnly
		} else if coordinate.Variant == VariantOracleEvidence {
			class = MatrixOracleContaminated
		}
		run := OfflineMatrixRunReceipt{
			Case: coordinate.Case, Variant: coordinate.Variant, EvidenceClass: class,
			PromotionReceiptSHA256:   strings.Repeat("1", 64),
			PublicRunAuthoritySHA256: strings.Repeat("2", 64),
			EpisodeSealSHA256:        strings.Repeat("3", 64),
			EvaluationArtifactSHA256: strings.Repeat("4", 64),
			OracleSHA256:             strings.Repeat("5", 64), OracleQuality: OracleOptimal,
			OracleReferenceDecisionCost: 4,
			TaskArchetype:               offlineScenarioTaskArchetype(coordinate.Case.Suite),
			GoalSuccess:                 true, ValidTerminalState: true, CausalAdmissionComplete: true,
			CleanDeskAvailable: true, CleanDeskQualified: true, CompetenceQualified: true,
			ModelCalls: 2, ModelVisibleBytes: 700, NativeInputTokens: 120,
			NativeOutputTokens: 40, ProviderTotalNanoseconds: 12,
			ProviderLoadNanoseconds: 2, ProviderPromptEvalNanoseconds: 4,
			ProviderEvalNanoseconds: 4, PolicyWallMilliseconds: 20,
			NativeUsageComplete: true, StationBudgetQualified: true,
			Reacquisitions: 1, ToolOperations: 8,
			InferenceStartedAt: registration.RegisteredAt.Add(time.Duration(index+1) * time.Second),
			InferenceExitedAt:  registration.RegisteredAt.Add(time.Duration(index+2) * time.Second),
			EvaluatorStartedAt: registration.RegisteredAt.Add(time.Duration(registration.RunCount+index+3) * time.Second),
			EvaluatorCompletedAt: registration.RegisteredAt.Add(
				time.Duration(registration.RunCount+index+4) * time.Second,
			),
		}
		if coordinate.Variant == VariantRawObservation ||
			coordinate.Variant == VariantFullTranscript ||
			coordinate.Variant == VariantTaskLedger ||
			coordinate.Variant == VariantLedgerWorkingSet ||
			coordinate.Variant == VariantLedgerProjection {
			run.ModelVisibleBytes, run.Reacquisitions, run.ToolOperations = 1_000, 2, 10
		} else if coordinate.Variant == VariantFullCognition {
			run.ModelVisibleBytes, run.Reacquisitions, run.ToolOperations = 500, 0, 5
		}
		runs = append(runs, run)
	}
	return runs
}

func matrixReceiptForGate(
	t *testing.T,
	registration OfflineMatrixPreregistration,
	runs []OfflineMatrixRunReceipt,
	gate OfflineMatrixGate,
) OfflineMatrixReceipt {
	t.Helper()
	sha, err := registration.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	lastInference := runs[len(runs)-1].InferenceExitedAt
	tournament, err := deriveOfflineMatrixTournament(registration, runs)
	if err != nil {
		t.Fatal(err)
	}
	bounds, err := deriveOfflineMatrixOracleBounds(registration, runs)
	if err != nil {
		t.Fatal(err)
	}
	return OfflineMatrixReceipt{
		Schema: OfflineMatrixReceiptSchemaV2, PreregistrationSHA256: sha,
		Runs: runs, DeterministicOracleBounds: bounds, Tournament: tournament,
		Gate: gate, LastInferenceExitedAt: lastInference,
		FirstEvaluatorStartedAt: runs[0].EvaluatorStartedAt,
		CompletedAt:             runs[len(runs)-1].EvaluatorCompletedAt,
		GateEvidenceQualified:   gate.Passed, PromotionEligible: false,
	}
}

func cloneMatrixRuns(runs []OfflineMatrixRunReceipt) []OfflineMatrixRunReceipt {
	return append([]OfflineMatrixRunReceipt{}, runs...)
}

func containsMatrixReason(reasons []string, fragment string) bool {
	for _, reason := range reasons {
		if strings.Contains(reason, fragment) {
			return true
		}
	}
	return false
}
