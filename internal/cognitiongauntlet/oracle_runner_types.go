package cognitiongauntlet

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/cognition"
)

type BaselinePurpose string

const BaselineWorldValidation BaselinePurpose = "world_validation_only"

type OracleRunRequest struct {
	Surface                 Surface              `json:"surface"`
	RatGeneration           RatGeneration        `json:"rat_generation"`
	RuntimeFingerprint      RuntimeFingerprint   `json:"runtime_fingerprint"`
	Repetition              int                  `json:"repetition"`
	Actor                   cognition.AttemptRef `json:"actor"`
	EpisodeSealPath         string               `json:"episode_seal_path"`
	EvaluationPath          string               `json:"evaluation_path"`
	OmnidexCommit           string               `json:"omnidex_commit,omitempty"`
	LedgerSchemaVersion     string               `json:"ledger_schema_version"`
	WorkingSetPolicyVersion string               `json:"working_set_policy_version"`
	ProjectionPolicyVersion string               `json:"projection_policy_version"`
}

type OracleBaselineResult struct {
	Purpose           BaselinePurpose         `json:"purpose"`
	Authority         PairedRunAuthority      `json:"authority"`
	Variant           VariantResult           `json:"variant"`
	Episode           SealedEpisode           `json:"episode"`
	Oracle            OracleManifest          `json:"oracle"`
	Evaluation        Evaluation              `json:"evaluation"`
	Efficiency        EfficiencyMetric        `json:"efficiency"`
	CausalAcquisition CausalAcquisitionReport `json:"causal_acquisition"`
}

func (request OracleRunRequest) Validate() error {
	if _, err := request.Surface.Version(); err != nil {
		return err
	}
	if err := request.RatGeneration.Validate(); err != nil {
		return err
	}
	if err := request.RuntimeFingerprint.Validate(); err != nil {
		return err
	}
	if request.Repetition <= 0 || request.Repetition > 10_000 {
		return fmt.Errorf("oracle baseline repetition is invalid")
	}
	if err := request.Actor.Validate(); err != nil {
		return err
	}
	if request.EpisodeSealPath == "" || request.EvaluationPath == "" ||
		request.EpisodeSealPath == request.EvaluationPath {
		return fmt.Errorf("oracle baseline requires distinct episode and evaluation paths")
	}
	if err := validateOracleOutputPaths(request.EpisodeSealPath, request.EvaluationPath); err != nil {
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
		return fmt.Errorf("oracle baseline Omnidex commit is invalid")
	}
	return nil
}

func validateOracleOutputPaths(episodePath, evaluationPath string) error {
	if filepath.Clean(episodePath) != episodePath || filepath.Clean(evaluationPath) != evaluationPath {
		return fmt.Errorf("oracle baseline output paths must be exact")
	}
	for _, path := range []string{episodePath, evaluationPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			return fmt.Errorf("oracle baseline output already exists or is inaccessible")
		}
	}
	episodeDirectory, episodeErr := os.Stat(filepath.Dir(episodePath))
	evaluationDirectory, evaluationErr := os.Stat(filepath.Dir(evaluationPath))
	if episodeErr != nil || evaluationErr != nil ||
		!episodeDirectory.IsDir() || !evaluationDirectory.IsDir() ||
		os.SameFile(episodeDirectory, evaluationDirectory) {
		return fmt.Errorf("oracle baseline requires separate existing episode and evaluation directories")
	}
	return nil
}

func (result OracleBaselineResult) Validate() error {
	if result.Purpose != BaselineWorldValidation {
		return fmt.Errorf("oracle baseline cannot be presented as cognition competence")
	}
	if result.Variant.Variant != VariantDeterministicOracle {
		return fmt.Errorf("oracle baseline result has a non-oracle variant")
	}
	if err := ValidateVariantEpisode(result.Variant, result.Episode); err != nil {
		return err
	}
	if result.Authority != result.Variant.Authority {
		return fmt.Errorf("oracle baseline result changed its paired authority")
	}
	if result.Evaluation.EpisodeSealSHA256 != result.Episode.SealSHA256 ||
		result.Evaluation.OracleSHA256 != result.Authority.OracleSHA256 ||
		!result.Evaluation.GoalSuccess || !result.Evaluation.ValidTerminalState {
		return fmt.Errorf("oracle baseline result has invalid evaluation authority")
	}
	if result.Oracle.Seed != result.Authority.Seed ||
		result.Oracle.GeneratorVersion != result.Authority.GeneratorVersion ||
		result.Oracle.OracleSHA256 != result.Authority.OracleSHA256 {
		return fmt.Errorf("oracle baseline result changed its private generator authority")
	}
	if err := result.CausalAcquisition.Validate(); err != nil {
		return err
	}
	if result.CausalAcquisition.AcquiredEvidence != result.CausalAcquisition.RequiredEvidence {
		return fmt.Errorf("deterministic oracle baseline lacks complete causal acquisition")
	}
	if result.CausalAcquisition.EpisodeSealSHA256 != result.Episode.SealSHA256 ||
		result.CausalAcquisition.OracleSHA256 != result.Oracle.OracleSHA256 ||
		result.CausalAcquisition.SurfaceVersion != result.Authority.SurfaceVersion {
		return fmt.Errorf("oracle baseline causal acquisition changed episode, oracle, or surface authority")
	}
	if result.Evaluation.CausalAcquisition == nil {
		return fmt.Errorf("oracle baseline evaluation omitted its causal acquisition report")
	}
	evaluationCausalSHA, err := digestJSON(*result.Evaluation.CausalAcquisition)
	if err != nil {
		return fmt.Errorf("hash evaluation causal acquisition: %w", err)
	}
	resultCausalSHA, err := digestJSON(result.CausalAcquisition)
	if err != nil || evaluationCausalSHA != resultCausalSHA {
		return fmt.Errorf("oracle baseline evaluation changed its causal acquisition report")
	}
	if err := ValidateEvaluationAuthority(result.Evaluation, result.Episode, result.Oracle); err != nil {
		return err
	}
	metric, err := result.Evaluation.EfficiencyMetric()
	if err != nil || metric != result.Efficiency {
		return fmt.Errorf("oracle baseline efficiency is inconsistent")
	}
	return nil
}
