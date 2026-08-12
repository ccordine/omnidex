package cognitiongauntlet

import (
	"context"
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
	Revision      cognition.WorldRevision
	Outcome       Outcome
	Terminal      *ablationPendingTerminal
	TerminalCause *ablationTerminalCause
	Resources     Resources
	Memory        MemoryMetrics
	Planning      PlanningMetrics
	FailureTrace  FailureTrace
	ContextBudget *ablationContextBudgetFailure
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
	publicAuthority, err := NewPublicRunAuthority(authority, request.Variant)
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
	bootstrap, err := attestAblationBrain(ctx, request.Client, brain)
	if err != nil {
		return AblationRunResult{}, err
	}
	attested := bootstrap.AttestedBrain
	frozen, err := request.RatGeneration.Fixed.Brain.attestedBrain()
	if err != nil || !sameFrozenBrain(attested, frozen) {
		return AblationRunResult{}, fmt.Errorf("live provider or host differs from frozen ablation authority")
	}
	activation, err := observeAblationProviderProcess(
		ctx, request.Client, attested, episode, request.Actor,
	)
	if err != nil {
		return AblationRunResult{}, err
	}
	activationAuthority, err := activation.Authority()
	if err != nil {
		return AblationRunResult{}, fmt.Errorf("derive ablation provider process activation authority: %w", err)
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
	policy, err := cognitionpolicy.New(
		request.Client, attested, activationAuthority, loader, journal,
	)
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
	bootstrapArtifact, bootstrapAuthority, err := prepareRuntimeBrainBootstrapEvidence(
		request.EpisodeSealPath, bootstrap, request.RatGeneration.Fixed.Brain,
	)
	if err != nil {
		return AblationRunResult{}, err
	}
	if err := appendRuntimeBrainBootstrapTrace(recorder, bootstrapAuthority); err != nil {
		return AblationRunResult{}, err
	}
	activationArtifact, activationEvidenceAuthority, err := prepareRuntimeProviderActivationEvidence(
		request.EpisodeSealPath, activation, request.RatGeneration.Fixed.Brain,
	)
	if err != nil {
		return AblationRunResult{}, err
	}
	if err := appendRuntimeProviderActivationTrace(recorder, activationEvidenceAuthority); err != nil {
		return AblationRunResult{}, err
	}
	startedAt := time.Now().UTC()
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
	if err := sealRuntimeBrainBootstrapEvidence(
		request.EpisodeSealPath, bootstrapArtifact, bootstrapAuthority,
		request.RatGeneration.Fixed.Brain,
	); err != nil {
		return AblationRunResult{}, err
	}
	if err := sealRuntimeProviderActivationEvidence(
		request.EpisodeSealPath, activationArtifact, activationEvidenceAuthority,
		request.RatGeneration.Fixed.Brain,
	); err != nil {
		return AblationRunResult{}, err
	}
	sealed, evidenceAuthority, err := finalizeAblationEpisode(
		request.EpisodeSealPath, request.EvidenceSealPath, startedAt,
		publicAuthority, recorder, state, journal, execution,
		bootstrapAuthority, activationEvidenceAuthority,
	)
	if err != nil {
		return AblationRunResult{}, err
	}
	return finishAblation(
		request, authority, fixture, sealed, evidenceAuthority, execution.FailureTrace,
	)
}
