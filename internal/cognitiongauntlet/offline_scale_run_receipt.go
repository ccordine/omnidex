package cognitiongauntlet

import (
	"fmt"
	"time"
)

func buildOfflineScaleRunReceipt(
	registration OfflineScalePreregistration,
	authority ScaleFamilyAuthority,
	artifact offlineScaleArtifacts,
) (OfflineScaleRunReceipt, error) {
	paired := PairedRunAuthority{
		Schema: PairedRunAuthoritySchemaV1, CaseID: artifact.current.ID,
		Suite: SuiteCombined, FixtureVersion: authority.FixtureVersion,
		GeneratorVersion: artifact.scaleEvidence.Family.GeneratorVersion,
		Seed:             registration.Plan.Seed, Scenario: artifact.bundle.Authority.Scenario,
		OracleSHA256: artifact.evaluation.OracleSHA256, SurfaceVersion: "symbolic.v1",
		ActionCatalogVersion: authority.ActionCatalogVersion,
		ActionCatalogSHA256:  authority.ActionCatalogSHA256,
		RatGeneration:        authority.RatGeneration, Budget: authority.Budget,
		Runtime: authority.Runtime, Repetition: artifact.current.Repetition,
	}
	if err := ValidatePublicRunAuthorityProjection(paired, artifact.bundle.Authority); err != nil {
		return OfflineScaleRunReceipt{}, err
	}
	causalComplete := artifact.evaluation.CausalAcquisition != nil &&
		artifact.evaluation.CausalAcquisition.Validate() == nil &&
		artifact.evaluation.CausalAcquisition.AcquiredEvidence ==
			artifact.evaluation.CausalAcquisition.RequiredEvidence
	cleanQualified := artifact.evaluation.CleanDesk != nil &&
		artifact.evaluation.CleanDesk.Validate() == nil &&
		artifact.evaluation.CleanDesk.ConcentrationQualified &&
		artifact.evaluation.CleanDesk.MissingCriticalBytes == 0
	resources := artifact.episode.Manifest.Resources
	result := OfflineScaleRunResult{
		Case: artifact.current, GeneratorVersion: paired.GeneratorVersion,
		Scenario: paired.Scenario, OracleSHA256: paired.OracleSHA256,
		EpisodeSealSHA256:    artifact.episode.SealSHA256,
		RelevantSurfaceBytes: artifact.scaleEvidence.RelevantSurfaceBytes,
		PeakContextBytes:     resources.PeakContextBytes, ModelDecisions: resources.ModelDecisions,
		RetrievalRounds:         resources.SearchOperations + resources.ReadOperations,
		GoalSuccess:             artifact.evaluation.GoalSuccess,
		ValidTerminalState:      artifact.evaluation.ValidTerminalState,
		CausalAdmissionComplete: causalComplete, CleanDeskQualified: cleanQualified,
	}
	result.CompetenceQualifiedSuccess = result.GoalSuccess && result.ValidTerminalState &&
		result.CausalAdmissionComplete && result.CleanDeskQualified
	publicSHA, err := artifact.bundle.Authority.SHA256()
	if err != nil {
		return OfflineScaleRunReceipt{}, err
	}
	run := OfflineScaleRunReceipt{
		Case: artifact.current, PromotionReceiptSHA256: artifact.promotionSHA,
		PublicAuthoritySHA256:    publicSHA,
		EvaluationArtifactSHA256: artifact.promotion.EvaluationArtifactSHA256,
		ScaleEvidenceSHA256:      artifact.scaleSHA, Result: result,
		InferenceStartedAt:   artifact.promotion.InferenceStartedAt,
		InferenceExitedAt:    artifact.promotion.InferenceExitedAt,
		EvaluatorStartedAt:   artifact.promotion.EvaluatorStartedAt,
		EvaluatorCompletedAt: artifact.promotion.CompletedAt,
	}
	return run, run.validate(artifact.current, registration.RegisteredAt, authority.Budget)
}

func (run OfflineScaleRunReceipt) validate(
	want OfflineScaleCase,
	registeredAt time.Time,
	budget RunBudget,
) error {
	if run.Case != want || run.Result.Case != want ||
		!validDigest(run.PromotionReceiptSHA256) || !validDigest(run.PublicAuthoritySHA256) ||
		!validDigest(run.EvaluationArtifactSHA256) || !validDigest(run.ScaleEvidenceSHA256) ||
		run.InferenceStartedAt.Before(registeredAt) ||
		run.InferenceExitedAt.Before(run.InferenceStartedAt) ||
		run.EvaluatorStartedAt.Before(run.InferenceExitedAt) ||
		run.EvaluatorCompletedAt.Before(run.EvaluatorStartedAt) {
		return fmt.Errorf("offline Scale run receipt authority is invalid")
	}
	return run.Result.validate(budget)
}

func (result OfflineScaleRunResult) validate(budget RunBudget) error {
	if err := requireExact(result.GeneratorVersion, "Scale generator version", 256); err != nil {
		return err
	}
	if result.Scenario.Validate() != nil || !validDigest(result.OracleSHA256) ||
		!validDigest(result.EpisodeSealSHA256) || result.RelevantSurfaceBytes <= 0 ||
		result.PeakContextBytes <= 0 || result.PeakContextBytes > int64(budget.ContextBytes) ||
		result.ModelDecisions <= 0 || result.ModelDecisions > budget.ModelCalls ||
		result.RetrievalRounds < 0 || result.RetrievalRounds > budget.ToolOperations ||
		result.GoalSuccess && !result.ValidTerminalState ||
		result.CompetenceQualifiedSuccess != (result.GoalSuccess && result.ValidTerminalState &&
			result.CausalAdmissionComplete && result.CleanDeskQualified) {
		return fmt.Errorf("offline Scale run result is invalid")
	}
	return nil
}
