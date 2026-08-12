package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
)

func (state *semanticReplayState) mapRecord(
	record queue.CognitionSealedTraceRecord,
	source cognitionreplay.SourceRecord,
) ([]semanticEventDraft, error) {
	switch record.Kind {
	case "transition":
		return state.mapTransition(record, source)
	case "action":
		return state.mapAction(record, source)
	case "action_event":
		return state.mapActionEvent(record, source)
	case "episode_progress":
		return state.mapEpisodeProgress(record, source)
	case "episode_progress_command":
		return state.mapEpisodeProgressCommand(record, source)
	case "cancellation_evidence":
		return state.mapCancellation(record, source)
	case "obligation_graph":
		return state.mapObligationGraph(record, source)
	case "policy_attempt":
		return state.mapPolicyAttempt(record, source)
	case "policy_result":
		return state.mapPolicyResult(record, source)
	case "policy_abandonment":
		return state.mapPolicyAbandonment(record, source)
	case "policy_timing":
		return state.mapPolicyTiming(record, source)
	case queue.CognitionTraceKindProposalMaterialization:
		return state.mapProposalMaterialization(record, source)
	case queue.CognitionTraceKindAcceptedFactMaterialization:
		return state.mapAcceptedFactMaterialization(record, source)
	case "reconciliation_command":
		return state.mapReconciliationCommand(record, source)
	case "reconciliation_receipt":
		return state.mapReconciliationReceipt(record, source)
	case "accepted_decision_recovery":
		return state.mapDecisionRecovery(record, source)
	case "belief_revision":
		return state.mapBeliefRevision(record, source)
	case "plan_revision":
		return state.mapPlanRevision(record, source)
	case "context_projection":
		return state.mapContextProjection(record, source)
	case "runtime_snapshot":
		return state.mapRuntimeSnapshot(record, source)
	case "working_set_event":
		return state.mapWorkingSetEvent(record, source)
	case "working_set_snapshot":
		return state.mapWorkingSetSnapshot(record, source)
	case "provider_brain_bootstrap":
		return state.mapProviderBrainBootstrap(record, source)
	case "provider_process_observation":
		return state.mapProviderProcessObservation(record, source)
	case "provider_activation_failure":
		return state.mapProviderActivationFailure(record, source)
	case "lifecycle_retirement":
		return state.mapLifecycleRetirement(record, source)
	case "policy_provider_generation_evidence", "policy_provider_response_capture",
		"policy_response_evidence":
		return state.mapOpaquePolicyEvidence(record, source)
	default:
		return nil, fmt.Errorf("unregistered frozen source kind %q", record.Kind)
	}
}
