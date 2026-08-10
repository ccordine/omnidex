package cognitiongauntlet

import "fmt"

func scoreAndSealOfflineScenarioEpisode(
	path string,
	generated generatedOfflineScenario,
	surfaceVersion string,
	episode SealedEpisode,
	evidence SymbolicEvaluationEvidence,
) (Evaluation, CausalAcquisitionReport, error) {
	if episode.Manifest.Resources.ModelCalls == 0 ||
		episode.Manifest.Variant == VariantDeterministicOracle {
		return Evaluation{}, CausalAcquisitionReport{}, fmt.Errorf(
			"offline scenario competence scoring requires a model episode",
		)
	}
	if evidence.ProjectionRelevance != nil {
		return Evaluation{}, CausalAcquisitionReport{}, fmt.Errorf(
			"offline scenario relevance must be derived by the private evaluator",
		)
	}
	authority := generated.evidenceAuthority()
	causal, err := measureOfflineCausalAcquisition(authority, episode, surfaceVersion)
	if err != nil {
		return Evaluation{}, CausalAcquisitionReport{}, err
	}
	if episode.Manifest.Outcome.GoalSatisfied &&
		causal.AcquiredEvidence != causal.RequiredEvidence {
		return Evaluation{}, CausalAcquisitionReport{}, fmt.Errorf(
			"successful offline scenario lacks complete causal acquisition: acquired %d of %d",
			causal.AcquiredEvidence, causal.RequiredEvidence,
		)
	}
	oracle, err := generated.oracleManifest()
	if err != nil {
		return Evaluation{}, CausalAcquisitionReport{}, err
	}
	relevance, err := buildOfflineProjectionRelevance(
		authority, oracle, episode, surfaceVersion,
	)
	if err != nil {
		return Evaluation{}, CausalAcquisitionReport{}, err
	}
	evidence.ProjectionRelevance = &relevance
	if !episode.Manifest.Outcome.GoalSatisfied && evidence.Failure == (FailureTrace{}) {
		evidence.Failure, err = deriveTerminalFailureTrace(episode)
		if err != nil {
			return Evaluation{}, CausalAcquisitionReport{}, err
		}
	}
	evaluation, err := ScoreSealedEpisode(episode, oracle, evidence)
	if err != nil {
		return Evaluation{}, CausalAcquisitionReport{}, err
	}
	evaluation.CausalAcquisition = &causal
	if err := ValidateEvaluationAuthority(evaluation, episode, oracle); err != nil {
		return Evaluation{}, CausalAcquisitionReport{}, err
	}
	if err := SealEvaluation(path, evaluation, episode, oracle); err != nil {
		return Evaluation{}, CausalAcquisitionReport{}, err
	}
	return evaluation, causal, nil
}
