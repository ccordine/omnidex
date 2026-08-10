package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

type AblationEvidenceClass string

const (
	AblationDevelopmentEvidence AblationEvidenceClass = "development_model_evidence"
	AblationIsolatedEvidence    AblationEvidenceClass = "isolated_process_model_evidence"
	AblationOracleContaminated  AblationEvidenceClass = "oracle_packet_contaminated"
	AblationBenchmarkOnly       AblationEvidenceClass = "benchmark_only_model_evidence"
)

type AblationRunRequest struct {
	Variant                 Variant
	Surface                 Surface
	RatGeneration           RatGeneration
	RuntimeFingerprint      RuntimeFingerprint
	Repetition              int
	Actor                   cognition.AttemptRef
	Client                  llm.Client
	EpisodeSealPath         string
	EvaluationPath          string
	OmnidexCommit           string
	LedgerSchemaVersion     string
	WorkingSetPolicyVersion string
	ProjectionPolicyVersion string
}

type AblationRunResult struct {
	EvidenceClass     AblationEvidenceClass   `json:"evidence_class"`
	PromotionEligible bool                    `json:"promotion_eligible"`
	Authority         PairedRunAuthority      `json:"authority"`
	Variant           VariantResult           `json:"variant"`
	Episode           SealedEpisode           `json:"episode"`
	Oracle            OracleManifest          `json:"oracle"`
	Evaluation        Evaluation              `json:"evaluation"`
	Efficiency        EfficiencyMetric        `json:"efficiency"`
	CausalAcquisition CausalAcquisitionReport `json:"causal_acquisition"`
}

func (request AblationRunRequest) Validate() error {
	if !executableAblation(request.Variant) {
		return fmt.Errorf("cognition ablation variant %q is not executable", request.Variant)
	}
	if _, err := request.Surface.Version(); err != nil {
		return err
	}
	if request.Variant == VariantRawShell && request.Surface != SurfaceFilesystem {
		return fmt.Errorf("raw-shell ablation requires the isolated filesystem surface")
	}
	if err := request.RatGeneration.Validate(); err != nil {
		return err
	}
	if err := request.RuntimeFingerprint.Validate(); err != nil {
		return err
	}
	if request.Repetition <= 0 || request.Repetition > 10_000 || nilRunDependency(request.Client) {
		return fmt.Errorf("cognition ablation requires repetition and an exact LLM client")
	}
	if err := request.Actor.Validate(); err != nil {
		return err
	}
	if err := validateRunOutputPaths(request.EpisodeSealPath, request.EvaluationPath); err != nil {
		return err
	}
	for label, value := range map[string]string{
		"Task Ledger schema version":        request.LedgerSchemaVersion,
		"Working Set policy version":        request.WorkingSetPolicyVersion,
		"Context Projection policy version": request.ProjectionPolicyVersion,
	} {
		if err := requireExact(value, label, 256); err != nil {
			return err
		}
	}
	if request.OmnidexCommit != "" && !validCommitIdentity(request.OmnidexCommit) {
		return fmt.Errorf("cognition ablation Omnidex commit is invalid")
	}
	return nil
}

func executableAblation(variant Variant) bool {
	switch variant {
	case VariantRawObservation, VariantFullTranscript, VariantTranscriptCompacted,
		VariantTaskLedger, VariantLedgerWorkingSet, VariantLedgerProjection,
		VariantOracleEvidence, VariantRawShell:
		return true
	default:
		return false
	}
}

func (result AblationRunResult) Validate() error {
	if !executableAblation(result.Variant.Variant) ||
		result.Authority != result.Variant.Authority {
		return fmt.Errorf("cognition ablation result authority is invalid")
	}
	validClass := false
	switch result.EvidenceClass {
	case AblationDevelopmentEvidence:
		validClass = !result.PromotionEligible
	case AblationIsolatedEvidence:
		validClass = result.PromotionEligible && result.Variant.Variant != VariantOracleEvidence &&
			result.Variant.Variant != VariantRawShell
	case AblationOracleContaminated:
		validClass = !result.PromotionEligible && result.Variant.Variant == VariantOracleEvidence
	case AblationBenchmarkOnly:
		validClass = !result.PromotionEligible && result.Variant.Variant == VariantRawShell
	}
	if !validClass {
		return fmt.Errorf("cognition ablation evidence class is invalid")
	}
	if err := ValidateVariantEpisode(result.Variant, result.Episode); err != nil {
		return err
	}
	if err := ValidateEvaluationAuthority(result.Evaluation, result.Episode, result.Oracle); err != nil {
		return err
	}
	metric, err := result.Evaluation.EfficiencyMetric()
	if err != nil || metric != result.Efficiency {
		return fmt.Errorf("cognition ablation efficiency is inconsistent")
	}
	if err := result.CausalAcquisition.Validate(); err != nil {
		return err
	}
	if result.CausalAcquisition.EpisodeSealSHA256 != result.Episode.SealSHA256 ||
		result.CausalAcquisition.OracleSHA256 != result.Oracle.OracleSHA256 ||
		result.CausalAcquisition.SurfaceVersion != result.Authority.SurfaceVersion {
		return fmt.Errorf("cognition ablation causal evidence changed episode, oracle, or surface authority")
	}
	return nil
}
