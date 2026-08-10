package cognitiongauntlet

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/contextbuilder"
)

type ablationProjectionLoader struct {
	current contextbuilder.Projection
}

func (loader *ablationProjectionLoader) LoadProjection(
	_ context.Context,
	ref cognition.ContextProjectionRef,
) (contextbuilder.Projection, error) {
	if loader == nil || loader.current.ID == "" || ref != ablationProjectionRef(loader.current) {
		return contextbuilder.Projection{}, fmt.Errorf("ablation projection loader rejected unbound authority")
	}
	return loader.current, nil
}

type ablationExecution struct {
	Revision     cognition.WorldRevision
	Outcome      Outcome
	Resources    Resources
	Memory       MemoryMetrics
	Planning     PlanningMetrics
	FailureTrace FailureTrace
}

func RunAblation(
	ctx context.Context,
	fixture MicrogauntletCase,
	request AblationRunRequest,
) (result AblationRunResult, runErr error) {
	if ctx == nil {
		return AblationRunResult{}, fmt.Errorf("cognition ablation context is nil")
	}
	if err := request.Validate(); err != nil {
		return AblationRunResult{}, err
	}
	authority, err := fixture.PairedAuthority(
		request.Surface, request.RatGeneration, request.Repetition, request.RuntimeFingerprint,
	)
	if err != nil {
		return AblationRunResult{}, err
	}
	episode, err := VariantEpisodeRef(authority, request.Variant)
	if err != nil {
		return AblationRunResult{}, err
	}
	brain, err := productionBrain(request.RatGeneration, authority.Budget.Station.MaxOutputTokens)
	if err != nil {
		return AblationRunResult{}, err
	}
	attested, err := cognitionpolicy.AttestBrain(ctx, request.Client, brain)
	if err != nil {
		return AblationRunResult{}, err
	}
	frozen, err := request.RatGeneration.Fixed.Brain.attestedBrain()
	if err != nil || !sameFrozenBrain(attested, frozen) {
		return AblationRunResult{}, fmt.Errorf("live provider or host differs from frozen ablation authority")
	}
	environment, closeEnvironment, err := newBenchmarkEnvironment(
		fixture, episode, request.Actor, request.Surface,
	)
	if err != nil {
		return AblationRunResult{}, err
	}
	goalEvaluator, ok := environment.(ablationGoalEvaluator)
	if !ok {
		return AblationRunResult{}, fmt.Errorf("local ablation environment lacks code-owned completion")
	}
	completion := localAblationCompletion{evaluator: goalEvaluator}
	defer func() {
		if closeErr := closeEnvironment(); closeErr != nil && runErr == nil {
			result = AblationRunResult{}
			runErr = fmt.Errorf("close cognition ablation environment: %w", closeErr)
		}
	}()
	state, err := newAblationState(
		request.Variant, episode, request.Actor, fixture.SealedEnvironmentScenario(),
		authority.Budget.WorkingSetBytes,
	)
	if err != nil {
		return AblationRunResult{}, err
	}
	journal := newAblationCallJournal()
	loader := &ablationProjectionLoader{}
	policy, err := cognitionpolicy.New(request.Client, attested, loader, journal)
	if err != nil {
		return AblationRunResult{}, err
	}
	template, err := ablationEpisodeTemplate(fixture, episode, request, authority)
	if err != nil {
		return AblationRunResult{}, err
	}
	recorder, err := NewEpisodeRecorder(template)
	if err != nil {
		return AblationRunResult{}, err
	}
	startedAt := time.Now()
	transition, err := environment.Start(ctx, authority.Scenario)
	if err != nil {
		return AblationRunResult{}, fmt.Errorf("start cognition ablation: %w", err)
	}
	if err := appendTransitionObservations(recorder, transition); err != nil {
		return AblationRunResult{}, err
	}
	if err := state.recordTransition(transition); err != nil {
		return AblationRunResult{}, err
	}
	privateEvidence := ContaminatedEvidencePacket{}
	if request.Variant == VariantOracleEvidence {
		generated := generatedOfflineScenario{
			spec: OfflineScenarioSpec{
				Schema: OfflineScenarioSpecSchemaV1, Kind: OfflineScenarioInitial,
				Initial: &fixture.spec,
			},
			scenario: fixture.SealedEnvironmentScenario(), public: fixture.PublicArtifact(),
			suite: Suite(fixture.spec.Generator.Suite), initial: &fixture,
		}
		privateEvidence, err = contaminatedEvidenceFor(generated)
		if err != nil {
			return AblationRunResult{}, err
		}
	}
	execution, err := executeAblation(
		ctx, authority.Budget, environment, completion,
		recorder, state, loader, journal, policy,
		privateEvidence, transition, startedAt,
	)
	if err != nil {
		return AblationRunResult{}, err
	}
	sealed, err := recorder.Seal(
		request.EpisodeSealPath, execution.Revision, execution.Outcome,
		execution.Resources, execution.Memory, execution.Planning, RecoveryMetrics{},
	)
	if err != nil {
		return AblationRunResult{}, err
	}
	return finishAblation(request, authority, fixture, sealed, execution.FailureTrace)
}

func executeAblation(
	ctx context.Context,
	budget RunBudget,
	environment cognition.Environment,
	completion ablationCompletionAuthority,
	recorder *EpisodeRecorder,
	state *ablationState,
	loader *ablationProjectionLoader,
	journal *ablationCallJournal,
	policy *cognitionpolicy.Policy,
	privateEvidence ContaminatedEvidencePacket,
	transition cognition.Transition,
	startedAt time.Time,
) (ablationExecution, error) {
	execution := ablationExecution{
		Revision: transition.Current,
		Planning: PlanningMetrics{ObligationsCreated: 1, PlanGenerations: 1},
	}
	for cycle := 1; cycle <= budget.RuntimeCycles; cycle++ {
		if transition.Terminal {
			return completeAblation(ctx, completion, state, execution, transition, startedAt, recorder)
		}
		if execution.Resources.ModelCalls >= budget.ModelCalls {
			return failAblation(execution, recorder, "resource_budget", "ablation-budget-model-calls",
				"The frozen model-call budget was exhausted.", startedAt, true)
		}
		context, err := state.context(uint32(cycle), privateEvidence)
		if err != nil {
			return ablationExecution{}, err
		}
		projection, snapshot, err := prepareAblationPolicyInput(
			state, budget, context, transition.Current, uint32(cycle), execution.Resources.ModelCalls,
		)
		if err != nil {
			if errors.Is(err, errAblationContextBudget) {
				id := fmt.Sprintf("ablation-context-budget-%03d", cycle)
				return failAblation(execution, recorder, "resource_budget", id, err.Error(), startedAt, true)
			}
			return ablationExecution{}, fmt.Errorf("prepare %s policy input: %w", state.variant, err)
		}
		loader.current = projection
		callIndex := execution.Resources.ModelCalls
		outcome, policyErr := policy.Decide(ctx, snapshot)
		if policyErr != nil && !registeredAblationPolicyFailure(policyErr) {
			return ablationExecution{}, policyErr
		}
		call, err := journal.completed(callIndex)
		if err != nil {
			return ablationExecution{}, err
		}
		if err := appendAblationPolicyTrace(recorder, projection, call, &execution.Resources); err != nil {
			return ablationExecution{}, err
		}
		if policyErr != nil {
			id := fmt.Sprintf("ablation-policy-failure-%03d", cycle)
			code := "model_policy"
			if errors.Is(policyErr, cognitionpolicy.ErrProviderUsageLimit) {
				code = "resource_budget"
			}
			return failAblation(execution, recorder, code, id, policyErr.Error(), startedAt, false)
		}
		requestAction := outcome.Decision.Action.Clone()
		if state.variant == VariantRawShell {
			requestAction, err = parseRawShellDecision(requestAction, state.catalog)
			if err != nil {
				id := fmt.Sprintf("ablation-shell-failure-%03d", cycle)
				return failAblation(execution, recorder, "model_policy", id, err.Error(), startedAt, false)
			}
		}
		terminal, err := applyAblationDecision(
			ctx, environment, recorder, state, &execution, outcome.Decision,
			requestAction, transition, cycle, budget,
		)
		if err != nil {
			return ablationExecution{}, err
		}
		transition = terminal
		if terminal.Terminal {
			return completeAblation(ctx, completion, state, execution, terminal, startedAt, recorder)
		}
		if execution.Outcome.Terminal {
			execution.Resources.WallMilliseconds = time.Since(startedAt).Milliseconds()
			return execution, nil
		}
	}
	return failAblation(execution, recorder, "resource_budget", "ablation-budget-cycles",
		"The frozen runtime-cycle budget was exhausted.", startedAt, true)
}

func registeredAblationPolicyFailure(err error) bool {
	for _, loud := range []error{
		cognitionpolicy.ErrProviderIdentity, cognitionpolicy.ErrCallJournal,
		cognitionpolicy.ErrCallIndeterminate, cognitionpolicy.ErrEnvelopeLimit,
		cognitionpolicy.ErrInputLimit, cognitionpolicy.ErrInvalidConfig,
		cognitionpolicy.ErrInvalidEvidence, cognitionpolicy.ErrInvalidProjection,
		cognitionpolicy.ErrProjectionMismatch, cognitionpolicy.ErrInvalidBrain,
	} {
		if errors.Is(err, loud) {
			return false
		}
	}
	return errors.Is(err, cognitionpolicy.ErrGeneration) ||
		errors.Is(err, cognitionpolicy.ErrInvalidDecision) ||
		errors.Is(err, cognitionpolicy.ErrResponseLimit) ||
		errors.Is(err, cognitionpolicy.ErrProviderUsageLimit) ||
		errors.Is(err, cognitionpolicy.ErrCallRejected)
}
