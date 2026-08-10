package cognitiongauntlet

import "fmt"

func ScoreMicrogauntletEpisode(
	fixture MicrogauntletCase,
	surfaceVersion string,
	episode SealedEpisode,
	evidence SymbolicEvaluationEvidence,
) (Evaluation, CausalAcquisitionReport, error) {
	if episode.Manifest.Resources.ModelCalls == 0 || episode.Manifest.Variant == VariantDeterministicOracle {
		return Evaluation{}, CausalAcquisitionReport{}, fmt.Errorf(
			"microgauntlet competence scoring requires a model episode",
		)
	}
	if evidence.ProjectionRelevance != nil {
		return Evaluation{}, CausalAcquisitionReport{}, fmt.Errorf(
			"microgauntlet relevance must be derived by the private post-seal evaluator",
		)
	}
	causal, err := MeasureCausalAcquisitionTrace(fixture, episode, surfaceVersion)
	if err != nil {
		return Evaluation{}, CausalAcquisitionReport{}, err
	}
	if episode.Manifest.Outcome.GoalSatisfied &&
		causal.AcquiredEvidence != causal.RequiredEvidence {
		return Evaluation{}, CausalAcquisitionReport{}, fmt.Errorf(
			"successful microgauntlet episode lacks complete causal acquisition: acquired %d of %d",
			causal.AcquiredEvidence, causal.RequiredEvidence,
		)
	}
	relevance, err := BuildPartialLabyrinthProjectionRelevance(fixture, episode, surfaceVersion)
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
	oracle, err := fixture.oracleManifest()
	if err != nil {
		return Evaluation{}, CausalAcquisitionReport{}, err
	}
	evaluation, err := ScoreSealedEpisode(episode, oracle, evidence)
	if err != nil {
		return Evaluation{}, CausalAcquisitionReport{}, err
	}
	evaluation.CausalAcquisition = &causal
	if err := ValidateEvaluationAuthority(evaluation, episode, oracle); err != nil {
		return Evaluation{}, CausalAcquisitionReport{}, err
	}
	return evaluation, causal, nil
}

func ScoreAndSealMicrogauntletEpisode(
	path string,
	fixture MicrogauntletCase,
	surfaceVersion string,
	episode SealedEpisode,
	evidence SymbolicEvaluationEvidence,
) (Evaluation, CausalAcquisitionReport, error) {
	evaluation, causal, err := ScoreMicrogauntletEpisode(
		fixture, surfaceVersion, episode, evidence,
	)
	if err != nil {
		return Evaluation{}, CausalAcquisitionReport{}, err
	}
	oracle, err := fixture.oracleManifest()
	if err != nil {
		return Evaluation{}, CausalAcquisitionReport{}, err
	}
	if err := SealEvaluation(path, evaluation, episode, oracle); err != nil {
		return Evaluation{}, CausalAcquisitionReport{}, err
	}
	return evaluation, causal, nil
}
