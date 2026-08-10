package cognitiongauntlet

import "fmt"

type GateResult struct {
	Passed  bool     `json:"passed"`
	Reasons []string `json:"reasons"`
}

type AbsoluteGateInput struct {
	HiddenOracleAccesses      int `json:"hidden_oracle_accesses"`
	UnauthorizedMutations     int `json:"unauthorized_mutations"`
	StaleEnvironmentActions   int `json:"stale_environment_actions"`
	StaleWorkerWrites         int `json:"stale_worker_writes"`
	UnboundContextProjections int `json:"unbound_context_projections"`
	ReplayDivergences         int `json:"replay_divergences"`
	ModelDeclaredCompletions  int `json:"model_declared_completions"`
}

func EvaluateAbsoluteGate(input AbsoluteGateInput) GateResult {
	reasons := []string{}
	checks := []struct {
		name  string
		value int
	}{
		{"hidden oracle accesses", input.HiddenOracleAccesses},
		{"unauthorized mutations", input.UnauthorizedMutations},
		{"stale environment actions", input.StaleEnvironmentActions},
		{"stale worker writes", input.StaleWorkerWrites},
		{"unbound Context Projections", input.UnboundContextProjections},
		{"deterministic replay divergences", input.ReplayDivergences},
		{"model-declared completions", input.ModelDeclaredCompletions},
	}
	for _, check := range checks {
		if check.value != 0 {
			reasons = append(reasons, fmt.Sprintf("%s=%d; require zero", check.name, check.value))
		}
	}
	return GateResult{Passed: len(reasons) == 0, Reasons: reasons}
}

type ContinuityGateInput struct {
	Episodes            int `json:"episodes"`
	CorrectWorld        int `json:"correct_world"`
	CorrectLedger       int `json:"correct_ledger"`
	CorrectWorkingSet   int `json:"correct_working_set"`
	IdenticalProjection int `json:"identical_projection"`
	DuplicateActions    int `json:"duplicate_actions"`
}

func EvaluateContinuityGate(input ContinuityGateInput) GateResult {
	reasons := []string{}
	if input.Episodes <= 0 {
		reasons = append(reasons, "continuity gate requires at least one episode")
	}
	for _, check := range []struct {
		name  string
		value int
	}{
		{"world", input.CorrectWorld}, {"Task Ledger", input.CorrectLedger},
		{"Working Set", input.CorrectWorkingSet}, {"Context Projection", input.IdenticalProjection},
	} {
		if check.value != input.Episodes {
			reasons = append(reasons, fmt.Sprintf("%s restoration=%d/%d; require 100%%", check.name, check.value, input.Episodes))
		}
	}
	if input.DuplicateActions != 0 {
		reasons = append(reasons, fmt.Sprintf("duplicate actions=%d; require zero", input.DuplicateActions))
	}
	return GateResult{Passed: len(reasons) == 0, Reasons: reasons}
}

type ScaleGateInput struct {
	WorldMultiplier   float64 `json:"world_multiplier"`
	ContextGrowth     float64 `json:"context_growth"`
	DecisionGrowth    float64 `json:"decision_growth"`
	SuccessLossPoints float64 `json:"success_loss_points"`
}

func EvaluateScaleGate(input ScaleGateInput) GateResult {
	reasons := []string{}
	if !finite(input.WorldMultiplier) || !finite(input.ContextGrowth) ||
		!finite(input.DecisionGrowth) || !finite(input.SuccessLossPoints) {
		return GateResult{Passed: false, Reasons: []string{"scale gate inputs must be finite"}}
	}
	if input.WorldMultiplier < 100 {
		reasons = append(reasons, "scale gate requires at least 100x world growth")
	}
	if input.ContextGrowth > 1.25 || input.ContextGrowth < 0 {
		reasons = append(reasons, "median context growth exceeds 25%")
	}
	if input.DecisionGrowth > 1.20 || input.DecisionGrowth < 0 {
		reasons = append(reasons, "median model-decision growth exceeds 20%")
	}
	if input.SuccessLossPoints > 5 || input.SuccessLossPoints < 0 {
		reasons = append(reasons, "success-rate loss exceeds five percentage points")
	}
	return GateResult{Passed: len(reasons) == 0, Reasons: reasons}
}

type PairedOutcome struct {
	CaseID           string `json:"case_id"`
	BaselineSuccess  bool   `json:"baseline_success"`
	CandidateSuccess bool   `json:"candidate_success"`
}

type PairedSummary struct {
	Cases       int `json:"cases"`
	Preserved   int `json:"preserved"`
	Regressions int `json:"regressions"`
	Rescues     int `json:"rescues"`
	Unresolved  int `json:"unresolved"`
}

type ExperienceReadinessGateInput struct {
	Episodes                 int `json:"episodes"`
	CompleteImmutableTraces  int `json:"complete_immutable_traces"`
	CompleteProjectionProofs int `json:"complete_projection_proofs"`
	KnownOutcomes            int `json:"known_outcomes"`
	KnownRuntimeVersions     int `json:"known_runtime_versions"`
	KnownModelVersions       int `json:"known_model_versions"`
	PostEvaluationArchetypes int `json:"post_evaluation_archetypes"`
	HiddenLabelExposures     int `json:"hidden_label_exposures"`
}

func EvaluateExperienceReadinessGate(input ExperienceReadinessGateInput) GateResult {
	reasons := []string{}
	if input.Episodes <= 0 {
		reasons = append(reasons, "experience readiness requires at least one sealed episode")
	}
	for _, check := range []struct {
		name  string
		value int
	}{
		{"immutable traces", input.CompleteImmutableTraces},
		{"projection proofs", input.CompleteProjectionProofs},
		{"known outcomes", input.KnownOutcomes},
		{"runtime versions", input.KnownRuntimeVersions},
		{"model versions", input.KnownModelVersions},
		{"post-evaluation archetypes", input.PostEvaluationArchetypes},
	} {
		if check.value != input.Episodes {
			reasons = append(reasons, fmt.Sprintf("%s=%d/%d; require complete", check.name, check.value, input.Episodes))
		}
	}
	if input.HiddenLabelExposures != 0 {
		reasons = append(reasons, fmt.Sprintf("hidden label exposures=%d; require zero", input.HiddenLabelExposures))
	}
	return GateResult{Passed: len(reasons) == 0, Reasons: reasons}
}

func SummarizePaired(outcomes []PairedOutcome) (PairedSummary, error) {
	if len(outcomes) == 0 {
		return PairedSummary{}, fmt.Errorf("paired cognition outcomes are required")
	}
	seen := make(map[string]struct{}, len(outcomes))
	summary := PairedSummary{Cases: len(outcomes)}
	for _, outcome := range outcomes {
		if err := requireExact(outcome.CaseID, "paired case ID", 256); err != nil {
			return PairedSummary{}, err
		}
		if _, duplicate := seen[outcome.CaseID]; duplicate {
			return PairedSummary{}, fmt.Errorf("paired cognition case %q appears more than once", outcome.CaseID)
		}
		seen[outcome.CaseID] = struct{}{}
		switch {
		case outcome.BaselineSuccess && outcome.CandidateSuccess:
			summary.Preserved++
		case outcome.BaselineSuccess && !outcome.CandidateSuccess:
			summary.Regressions++
		case !outcome.BaselineSuccess && outcome.CandidateSuccess:
			summary.Rescues++
		default:
			summary.Unresolved++
		}
	}
	return summary, nil
}
