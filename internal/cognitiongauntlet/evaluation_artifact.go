package cognitiongauntlet

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

func ValidateEvaluationAuthority(
	evaluation Evaluation,
	episode SealedEpisode,
	oracle OracleManifest,
) error {
	if err := evaluation.Validate(); err != nil {
		return err
	}
	if err := episode.Validate(); err != nil {
		return err
	}
	if err := oracle.Validate(); err != nil {
		return err
	}
	if evaluation.EpisodeSealSHA256 != episode.SealSHA256 ||
		evaluation.OracleSHA256 != oracle.OracleSHA256 ||
		evaluation.Seed != oracle.Seed ||
		episode.Manifest.Scenario.ID != oracle.ScenarioID ||
		episode.Manifest.Scenario.SHA256 != oracle.PublicSHA256 {
		return fmt.Errorf("cognition evaluation does not bind the sealed episode and private oracle")
	}
	if evaluation.Quality != oracle.Quality || evaluation.TaskArchetype != oracle.TaskArchetype {
		return fmt.Errorf("cognition evaluation changed private oracle metadata")
	}
	if episode.Manifest.Resources.ModelCalls == 0 {
		if evaluation.CleanDesk != nil {
			return fmt.Errorf("non-model cognition evaluation cannot claim clean-desk metrics")
		}
	} else if evaluation.CleanDesk == nil ||
		evaluation.CleanDesk.EpisodeSealSHA256 != episode.SealSHA256 ||
		evaluation.CleanDesk.OracleSHA256 != oracle.OracleSHA256 ||
		len(evaluation.CleanDesk.Calls) != episode.Manifest.Resources.ModelCalls {
		return fmt.Errorf("model cognition evaluation lacks sealed clean-desk authority")
	}
	if evaluation.CausalAcquisition != nil &&
		(evaluation.CausalAcquisition.EpisodeSealSHA256 != episode.SealSHA256 ||
			evaluation.CausalAcquisition.OracleSHA256 != oracle.OracleSHA256) {
		return fmt.Errorf("cognition evaluation causal acquisition changed episode or oracle authority")
	}
	wantReference := oracle.WitnessCost
	if oracle.Quality == OracleOptimal {
		wantReference = *oracle.OptimalCost
	}
	if evaluation.ReferenceDecisionCost != wantReference {
		return fmt.Errorf("cognition evaluation reference cost does not match its oracle quality")
	}
	return nil
}

func SealEvaluation(
	path string,
	evaluation Evaluation,
	episode SealedEpisode,
	oracle OracleManifest,
) error {
	if err := ValidateEvaluationAuthority(evaluation, episode, oracle); err != nil {
		return err
	}
	return sealScenarioArtifact(path, evaluation, "cognition evaluation")
}

func LoadEvaluation(path string) (Evaluation, error) {
	evaluation, _, err := LoadEvaluationArtifact(path)
	return evaluation, err
}

func LoadEvaluationArtifact(path string) (Evaluation, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Evaluation{}, "", fmt.Errorf("read cognition evaluation: %w", err)
	}
	if len(raw) == 0 || len(raw) > 64*1024+1 {
		return Evaluation{}, "", fmt.Errorf("cognition evaluation artifact size is invalid")
	}
	var evaluation Evaluation
	if err := decodeStrictJSON(raw, &evaluation, "cognition evaluation"); err != nil {
		return Evaluation{}, "", err
	}
	if err := evaluation.Validate(); err != nil {
		return Evaluation{}, "", err
	}
	digest := sha256.Sum256(raw)
	return evaluation, hex.EncodeToString(digest[:]), nil
}
