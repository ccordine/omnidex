package cognitiongauntlet

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"reflect"

	"github.com/gryph/omnidex/internal/labyrinth"
)

const maxOfflineMatrixArtifactBytes = 4 * 1024 * 1024

// VerifiedOfflineMatrixReceipt cannot be constructed by a caller. It exists
// only after every coordinate artifact has been re-opened and revalidated.
type VerifiedOfflineMatrixReceipt struct {
	receipt OfflineMatrixReceipt
}

func (verified VerifiedOfflineMatrixReceipt) RunCount() int {
	return len(verified.receipt.Runs)
}

func (verified VerifiedOfflineMatrixReceipt) PromotionEligible() bool {
	return verified.receipt.PromotionEligible
}

func (verified VerifiedOfflineMatrixReceipt) GateEvidenceQualified() bool {
	return verified.receipt.GateEvidenceQualified
}

func (verified VerifiedOfflineMatrixReceipt) ReleaseCoverageQualified() bool {
	return verified.receipt.ReleaseCoverageQualified
}

func (verified VerifiedOfflineMatrixReceipt) Receipt() OfflineMatrixReceipt {
	copy := verified.receipt
	copy.Runs = append([]OfflineMatrixRunReceipt{}, verified.receipt.Runs...)
	copy.DeterministicOracleBounds = append(
		[]OfflineMatrixOracleBound{}, verified.receipt.DeterministicOracleBounds...,
	)
	copy.Tournament.Rounds = append(
		[]OfflineMatrixRoundResult{}, verified.receipt.Tournament.Rounds...,
	)
	copy.Gate.Reasons = cloneMatrixSlice(verified.receipt.Gate.Reasons)
	return copy
}

func buildOfflineMatrixRunReceipt(
	coordinate OfflineMatrixCase,
	variant Variant,
	promotion OfflinePromotionReceipt,
	promotionArtifactSHA256 string,
	episode SealedEpisode,
	evaluation Evaluation,
) (OfflineMatrixRunReceipt, error) {
	if err := promotion.Validate(); err != nil {
		return OfflineMatrixRunReceipt{}, err
	}
	if err := evaluation.Validate(); err != nil {
		return OfflineMatrixRunReceipt{}, err
	}
	if promotion.EpisodeSealSHA256 != episode.SealSHA256 ||
		promotion.EvaluationArtifactSHA256 == "" ||
		evaluation.EpisodeSealSHA256 != episode.SealSHA256 ||
		!validDigest(promotionArtifactSHA256) {
		return OfflineMatrixRunReceipt{}, fmt.Errorf("offline matrix run artifacts changed authority")
	}
	class := MatrixIsolatedProcess
	if variant == VariantRawShell {
		class = MatrixBenchmarkOnly
	} else if variant == VariantOracleEvidence {
		class = MatrixOracleContaminated
	}
	run := OfflineMatrixRunReceipt{
		Case: coordinate, Variant: variant, EvidenceClass: class,
		PromotionReceiptSHA256:      promotionArtifactSHA256,
		PublicRunAuthoritySHA256:    promotion.PublicRunAuthoritySHA256,
		EpisodeSealSHA256:           episode.SealSHA256,
		EvaluationArtifactSHA256:    promotion.EvaluationArtifactSHA256,
		OracleSHA256:                evaluation.OracleSHA256,
		OracleQuality:               evaluation.Quality,
		OracleReferenceDecisionCost: evaluation.ReferenceDecisionCost,
		TaskArchetype:               evaluation.TaskArchetype,
		GoalSuccess:                 evaluation.GoalSuccess, ValidTerminalState: evaluation.ValidTerminalState,
		ModelCalls:                    episode.Manifest.Resources.ModelCalls,
		NativeInputTokens:             episode.Manifest.Resources.InputTokens,
		NativeOutputTokens:            episode.Manifest.Resources.OutputTokens,
		ProviderTotalNanoseconds:      episode.Manifest.Resources.ProviderTotalNanoseconds,
		ProviderLoadNanoseconds:       episode.Manifest.Resources.ProviderLoadNanoseconds,
		ProviderPromptEvalNanoseconds: episode.Manifest.Resources.ProviderPromptEvalNanoseconds,
		ProviderEvalNanoseconds:       episode.Manifest.Resources.ProviderEvalNanoseconds,
		PolicyWallMilliseconds:        episode.Manifest.Resources.PolicyWallMilliseconds,
		Reacquisitions:                episode.Manifest.Memory.Reacquisitions,
		ToolOperations:                episode.Manifest.Resources.ToolOperations,
		InferenceStartedAt:            promotion.InferenceStartedAt,
		InferenceExitedAt:             promotion.InferenceExitedAt,
		EvaluatorStartedAt:            promotion.EvaluatorStartedAt,
		EvaluatorCompletedAt:          promotion.CompletedAt,
	}
	if evaluation.CausalAcquisition != nil {
		report := evaluation.CausalAcquisition
		run.CausalAdmissionComplete = report.Validate() == nil &&
			report.AcquiredEvidence == report.RequiredEvidence
	}
	if evaluation.CleanDesk != nil {
		run.CleanDeskAvailable = true
		run.ModelVisibleBytes = evaluation.CleanDesk.TotalModelVisibleBytes
		run.MissingCriticalRefs = evaluation.CleanDesk.MissingCriticalRefs
		run.NativeUsageComplete = evaluation.CleanDesk.NativeUsageComplete
		run.StationBudgetQualified = evaluation.CleanDesk.BudgetQualified
		run.CleanDeskQualified = evaluation.CleanDesk.ConcentrationQualified &&
			evaluation.CleanDesk.MissingCriticalRefs == 0 && run.NativeUsageComplete &&
			run.StationBudgetQualified
	}
	run.CompetenceQualified = run.GoalSuccess && run.ValidTerminalState &&
		run.CausalAdmissionComplete && run.CleanDeskQualified
	return run, nil
}

func loadOfflinePromotionReceiptArtifact(
	path string,
) (OfflinePromotionReceipt, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return OfflinePromotionReceipt{}, "", fmt.Errorf("read offline promotion receipt: %w", err)
	}
	if len(raw) == 0 || len(raw) > maxOfflineMatrixArtifactBytes {
		return OfflinePromotionReceipt{}, "", fmt.Errorf("offline promotion receipt size is invalid")
	}
	var receipt OfflinePromotionReceipt
	if err := decodeStrictJSON(raw, &receipt, "offline promotion receipt"); err != nil {
		return OfflinePromotionReceipt{}, "", err
	}
	if err := receipt.Validate(); err != nil {
		return OfflinePromotionReceipt{}, "", err
	}
	digest := sha256.Sum256(raw)
	return receipt, hex.EncodeToString(digest[:]), nil
}

func SealOfflineMatrixReceipt(
	path string,
	receipt OfflineMatrixReceipt,
	registration OfflineMatrixPreregistration,
) error {
	if err := receipt.Validate(registration); err != nil {
		return err
	}
	return sealScenarioArtifact(path, receipt, "offline cognition matrix receipt")
}

func LoadOfflineMatrixReceipt(
	path string,
	registration OfflineMatrixPreregistration,
) (OfflineMatrixReceipt, error) {
	var receipt OfflineMatrixReceipt
	if err := loadStrictJSONFile(path, &receipt, "offline cognition matrix receipt"); err != nil {
		return OfflineMatrixReceipt{}, err
	}
	if err := receipt.Validate(registration); err != nil {
		return OfflineMatrixReceipt{}, err
	}
	return receipt, nil
}

func LoadVerifiedOfflineMatrixReceipt(
	config OfflineMatrixConfig,
) (VerifiedOfflineMatrixReceipt, error) {
	registration, err := LoadOfflineMatrixPreregistration(config.Paths().Preregistration)
	if err != nil {
		return VerifiedOfflineMatrixReceipt{}, err
	}
	receipt, err := LoadOfflineMatrixReceipt(config.Paths().Receipt, registration)
	if err != nil {
		return VerifiedOfflineMatrixReceipt{}, err
	}
	if err := VerifyOfflineMatrixReceipt(config, receipt); err != nil {
		return VerifiedOfflineMatrixReceipt{}, err
	}
	return VerifiedOfflineMatrixReceipt{receipt: receipt}, nil
}

func VerifyOfflineMatrixReceipt(
	config OfflineMatrixConfig,
	receipt OfflineMatrixReceipt,
) error {
	registration, err := LoadOfflineMatrixPreregistration(config.Paths().Preregistration)
	if err != nil {
		return err
	}
	if err := receipt.Validate(registration); err != nil {
		return err
	}
	surfaceVersion, err := config.Plan.Surface.Version()
	if err != nil {
		return err
	}
	for index, coordinate := range matrixCoordinates(registration) {
		runConfig, err := config.derivedRunConfig(coordinate.Case, coordinate.Variant)
		if err != nil {
			return err
		}
		paths := runConfig.Paths()
		bundle, err := LoadPublicInferenceBundle(paths.PublicBundle)
		if err != nil {
			return err
		}
		promotion, promotionSHA, err := loadOfflinePromotionReceiptArtifact(paths.Receipt)
		if err != nil {
			return err
		}
		evaluation, err := promotion.VerifyEvaluationArtifact(paths.Evaluation)
		if err != nil {
			return err
		}
		episode, err := LoadSealedEpisode(paths.Episode)
		if err != nil {
			return err
		}
		publicSHA256, err := bundle.Authority.SHA256()
		if err != nil {
			return err
		}
		if bundle.Authority.Variant != coordinate.Variant ||
			bundle.Authority.Repetition != coordinate.Case.Repetition ||
			bundle.Authority.RatGeneration != config.RatGeneration ||
			bundle.Authority.Budget != config.Budget ||
			bundle.Authority.Runtime != config.RuntimeFingerprint ||
			bundle.Authority.SurfaceVersion != surfaceVersion ||
			promotion.PublicRunAuthoritySHA256 != publicSHA256 ||
			evaluation.Seed != coordinate.Case.Seed ||
			evaluation.TaskArchetype != offlineScenarioTaskArchetype(coordinate.Case.Suite) {
			return fmt.Errorf("offline matrix coordinate %d changed its sealed run authority", index+1)
		}
		if err := validatePublicInferenceEpisode(bundle, episode); err != nil {
			return err
		}
		rebuilt, err := buildOfflineMatrixRunReceipt(
			coordinate.Case, coordinate.Variant, promotion, promotionSHA, episode, evaluation,
		)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(receipt.Runs[index], rebuilt) {
			return fmt.Errorf("offline matrix run %d differs from its exact artifacts", index+1)
		}
	}
	return nil
}

func offlineScenarioTaskArchetype(suite Suite) string {
	switch suite {
	case SuiteRetrieve:
		return string(labyrinth.ArchetypeRetrieve)
	case SuiteRecall:
		return string(labyrinth.ArchetypeRecall)
	case SuiteUnlock:
		return string(labyrinth.ArchetypeUnlock)
	case SuiteMutate:
		return string(labyrinth.ArchetypeMutate)
	case SuiteCombined:
		return string(labyrinth.ArchetypeCombined)
	case SuiteTraverse:
		return "partial-map-backtracking"
	case SuiteBind:
		return "distant-evidence-binding"
	case SuiteRevise:
		return "contradiction-revision-replanning"
	case SuiteOrder:
		return "ordered-irreversible-actions"
	default:
		return ""
	}
}
