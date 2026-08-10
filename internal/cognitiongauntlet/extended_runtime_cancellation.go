package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/gryph/omnidex/internal/queue"
)

func finishExtendedCanceledRuntime(
	ctx context.Context,
	generated labyrinth.ExtendedCase,
	request ExtendedRuntimeRunRequest,
	authority PairedRunAuthority,
	episode cognition.EpisodeRef,
	run cognitionruntime.RunResult,
	components fullRuntimeComponents,
	prerequisiteSHA256 string,
) (ExtendedRuntimeReceipt, error) {
	trace, err := readProductionTrace(ctx, components.repository, episode.ID)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	if trace.Header.Seal.Outcome != queue.CognitionEpisodeCanceled {
		return ExtendedRuntimeReceipt{}, fmt.Errorf("extended runtime cancellation did not seal the episode")
	}
	metrics, err := validateExtendedCanceledTrace(
		authority, request, episode, generated.ExecutionScenario().Ref(), trace,
	)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	cost, actions, failures, err := replayExtendedActionTrace(
		ctx, generated.ExecutionScenario(), episode, request.Attempt, trace, false,
	)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	if metrics.Resources.EnvironmentActions != actions {
		return ExtendedRuntimeReceipt{}, fmt.Errorf("canceled extended runtime counters differ from its sealed trace")
	}
	if run.Terminal.RecoveredProgress {
		if run.PolicyCalls != 0 || run.EnvironmentActions != 0 {
			return ExtendedRuntimeReceipt{}, fmt.Errorf("canceled extended runtime replay performed work")
		}
	} else if metrics.Resources.ModelCalls != int(run.PolicyCalls) ||
		metrics.Resources.EnvironmentActions != int(run.EnvironmentActions) {
		return ExtendedRuntimeReceipt{}, fmt.Errorf("canceled extended runtime counters differ from its live run")
	}
	code := cognitionruntime.CancellationCode(metrics.Outcome.FailureCode)
	evidenceClass, err := extendedClientEvidenceClass(request.Client)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	proofs, err := extendedRuntimeProofs(generated, trace)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	witnessCost := 0
	for _, action := range generated.PrivateOracle().Witness {
		witnessCost += action.Cost
	}
	receipt := ExtendedRuntimeReceipt{
		Schema: ExtendedRuntimeReceiptSchemaV1, Authority: authority,
		EpisodeID: string(episode.ID), Seal: trace.Header.Seal,
		PolicyCalls: uint32(metrics.Resources.ModelCalls), EnvironmentActions: uint32(actions),
		FailedActions: uint32(failures), EvidenceClass: evidenceClass,
		PromotionEligible: false, CancellationCode: code,
		CostBaseline: extendedCostWitnessOnly, ActualCost: cost, WitnessCost: witnessCost,
		Proofs: proofs, RevisionTraceSHA256: "",
		PrerequisiteBundleSHA256: prerequisiteSHA256,
	}
	receipt.ReceiptSHA256, err = extendedRuntimeReceiptSHA(receipt)
	if err != nil {
		return ExtendedRuntimeReceipt{}, err
	}
	return receipt, receipt.Validate()
}

func validateExtendedCanceledTrace(
	authority PairedRunAuthority,
	request ExtendedRuntimeRunRequest,
	episode cognition.EpisodeRef,
	scenario cognition.ScenarioRef,
	trace productionTrace,
) (productionTraceMetrics, error) {
	authoritySHA, err := publicRunAuthoritySHA256(authority, VariantFullCognition)
	if err != nil {
		return productionTraceMetrics{}, err
	}
	recorder, err := NewEpisodeRecorder(EpisodeManifest{
		Schema: EpisodeManifestSchemaV1, EpisodeID: episode.ID,
		Scenario:                 scenario,
		PublicRunAuthoritySHA256: authoritySHA, Variant: VariantFullCognition,
		RatGeneration: request.RatGeneration, StationBudget: authority.Budget.Station,
	})
	if err != nil {
		return productionTraceMetrics{}, err
	}
	metrics, err := appendProductionTrace(recorder, trace, RecoveryMetrics{}, []RestartTrace{})
	if err != nil {
		return productionTraceMetrics{}, err
	}
	if metrics.Outcome.GoalSatisfied || !metrics.Outcome.Terminal ||
		(metrics.Outcome.FailureCode != string(cognitionruntime.CancellationPolicyFailure) &&
			metrics.Outcome.FailureCode != string(cognitionruntime.CancellationRunBudgetExhausted)) {
		return productionTraceMetrics{}, fmt.Errorf("extended runtime cancellation trace changed its registered outcome")
	}
	return metrics, nil
}
