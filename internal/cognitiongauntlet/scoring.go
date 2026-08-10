package cognitiongauntlet

import "fmt"

type SymbolicEvaluationEvidence struct {
	GoalPredicateSatisfied bool                         `json:"goal_predicate_satisfied"`
	ValidTerminalState     bool                         `json:"valid_terminal_state"`
	ActualDecisionCost     int64                        `json:"actual_decision_cost"`
	Failure                FailureTrace                 `json:"failure"`
	PrivateAuthorityRefs   []string                     `json:"private_authority_refs"`
	ProjectionRelevance    *ProjectionRelevanceEvidence `json:"projection_relevance,omitempty"`
}

func ScoreSealedEpisode(
	episode SealedEpisode,
	oracle OracleManifest,
	evidence SymbolicEvaluationEvidence,
) (Evaluation, error) {
	if err := episode.Validate(); err != nil {
		return Evaluation{}, err
	}
	if err := oracle.Validate(); err != nil {
		return Evaluation{}, err
	}
	if episode.Manifest.Scenario.ID != oracle.ScenarioID ||
		episode.Manifest.Scenario.SHA256 != oracle.PublicSHA256 {
		return Evaluation{}, fmt.Errorf("symbolic evaluator received an oracle for another episode")
	}
	if evidence.ActualDecisionCost < 0 {
		return Evaluation{}, fmt.Errorf("symbolic evaluator decision cost cannot be negative")
	}
	if episode.Manifest.Outcome.GoalSatisfied && !evidence.GoalPredicateSatisfied {
		return Evaluation{}, fmt.Errorf("episode claimed goal success that the symbolic evaluator rejected")
	}
	goalSuccess := evidence.GoalPredicateSatisfied && evidence.ValidTerminalState &&
		episode.Manifest.Outcome.Terminal && episode.Manifest.Outcome.GoalSatisfied
	evaluation := Evaluation{
		Schema: EvaluationSchemaV1, EpisodeSealSHA256: episode.SealSHA256,
		OracleSHA256: oracle.OracleSHA256, Seed: oracle.Seed,
		EvaluatorVersion: episode.Manifest.RatGeneration.Fixed.EvaluatorVersion,
		TaskArchetype:    oracle.TaskArchetype, Quality: oracle.Quality,
		GoalSuccess: goalSuccess, ValidTerminalState: evidence.ValidTerminalState,
		ActualDecisionCost:    evidence.ActualDecisionCost,
		ReferenceDecisionCost: oracleReferenceCost(oracle),
	}
	if episode.Manifest.Resources.ModelCalls > 0 {
		if evidence.ProjectionRelevance == nil {
			return Evaluation{}, fmt.Errorf("model episode evaluation requires private projection relevance authority")
		}
		cleanDesk, err := EvaluateCleanDesk(episode, oracle, *evidence.ProjectionRelevance)
		if err != nil {
			return Evaluation{}, err
		}
		evaluation.CleanDesk = &cleanDesk
	} else if evidence.ProjectionRelevance != nil {
		return Evaluation{}, fmt.Errorf("non-model episode cannot claim clean-desk metrics")
	}
	if !goalSuccess {
		trace := evidence.Failure
		if evidence.GoalPredicateSatisfied &&
			(!episode.Manifest.Outcome.Terminal || !episode.Manifest.Outcome.GoalSatisfied) {
			trace.GoalPredicateTrue = true
			trace.TerminalRecorded = false
		}
		attribution, err := AttributeFailure(trace)
		if err != nil {
			return Evaluation{}, err
		}
		if err := ValidateAttributionReferences(episode, attribution, evidence.PrivateAuthorityRefs); err != nil {
			return Evaluation{}, err
		}
		evaluation.Attribution = &attribution
	}
	if err := ValidateEvaluationAuthority(evaluation, episode, oracle); err != nil {
		return Evaluation{}, err
	}
	return evaluation, nil
}

func ValidateAttributionReferences(
	episode SealedEpisode,
	attribution FailureAttribution,
	privateAuthorityRefs []string,
) error {
	if err := episode.Validate(); err != nil {
		return err
	}
	if err := attribution.Validate(); err != nil {
		return err
	}
	known := make(map[string]struct{}, len(episode.Manifest.Trace)+len(privateAuthorityRefs))
	for _, entry := range episode.Manifest.Trace {
		known[entry.ID] = struct{}{}
	}
	for index, ref := range privateAuthorityRefs {
		if err := requireExact(ref, fmt.Sprintf("private attribution authority %d", index+1), 512); err != nil {
			return err
		}
		if _, duplicate := known[ref]; duplicate {
			return fmt.Errorf("attribution authority reference %q is duplicated", ref)
		}
		known[ref] = struct{}{}
	}
	for _, ref := range attribution.TraceRefs {
		if _, exists := known[ref]; !exists {
			return fmt.Errorf("failure attribution cites unknown evidence %q", ref)
		}
	}
	return nil
}

func ScoreAndSealEpisode(
	path string,
	episode SealedEpisode,
	oracle OracleManifest,
	evidence SymbolicEvaluationEvidence,
) (Evaluation, error) {
	evaluation, err := ScoreSealedEpisode(episode, oracle, evidence)
	if err != nil {
		return Evaluation{}, err
	}
	if err := SealEvaluation(path, evaluation, episode, oracle); err != nil {
		return Evaluation{}, err
	}
	return evaluation, nil
}

func oracleReferenceCost(oracle OracleManifest) int64 {
	if oracle.Quality == OracleOptimal {
		return *oracle.OptimalCost
	}
	return oracle.WitnessCost
}
