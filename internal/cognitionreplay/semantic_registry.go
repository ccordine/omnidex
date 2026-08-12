package cognitionreplay

import (
	"fmt"
	"sort"
)

type semanticSourceRule struct {
	disposition MappingDisposition
	events      map[EventKind]struct{}
}

func semanticRule(disposition MappingDisposition, events ...EventKind) semanticSourceRule {
	allowed := make(map[EventKind]struct{}, len(events))
	for _, event := range events {
		allowed[event] = struct{}{}
	}
	return semanticSourceRule{disposition: disposition, events: allowed}
}

var frozenSemanticSourceRegistry = map[string]semanticSourceRule{
	"accepted_decision_recovery": semanticRule(MappingSemanticTyped,
		EventEpisodeRestarted),
	"action": semanticRule(MappingSemanticTyped,
		EventActionSelected, EventFailureRecorded),
	"action_event": semanticRule(MappingSemanticTyped,
		EventEvidenceAcquired),
	"accepted_fact_materialization": semanticRule(MappingSemanticTyped,
		EventEvidenceAcquired, EventFactAccepted),
	"belief_revision": semanticRule(MappingSemanticTyped,
		EventHypothesisRejected),
	"cancellation_evidence": semanticRule(MappingSemanticTyped,
		EventEpisodeCanceled, EventFailureRecorded, EventEpisodeSealed),
	"context_projection": semanticRule(MappingSemanticTyped,
		EventContextProjected),
	"episode_progress": semanticRule(MappingSemanticTyped,
		EventObligationChanged, EventGoalSatisfied, EventGoalFailed,
		EventFailureRecorded, EventEpisodeSealed),
	"episode_progress_command": semanticRule(MappingSemanticTyped,
		EventObligationChanged),
	"lifecycle_retirement": semanticRule(MappingSemanticTyped,
		EventLeaseChanged),
	"obligation_graph": semanticRule(MappingSemanticTyped,
		EventGoalActivated, EventObligationCreated, EventObligationChanged),
	"plan_revision": semanticRule(MappingSemanticTyped,
		EventPlanRevised),
	"policy_abandonment": semanticRule(MappingSemanticTyped,
		EventModelCallDisposition, EventStaleWriteRejected),
	"policy_attempt": semanticRule(MappingSemanticTyped,
		EventModelCalled),
	"policy_provider_generation_evidence": semanticRule(MappingSemanticOpaque,
		EventEvidenceAcquired),
	"policy_provider_response_capture": semanticRule(MappingSemanticOpaque,
		EventEvidenceAcquired),
	"policy_response_evidence": semanticRule(MappingSemanticOpaque,
		EventEvidenceAcquired),
	"policy_result": semanticRule(MappingSemanticTyped,
		EventProviderRequestDisposition, EventModelCallDisposition,
		EventDecisionAccepted, EventDecisionRejected, EventFailureRecorded),
	"policy_timing": semanticRule(MappingSemanticOpaque,
		EventEvidenceAcquired),
	"proposal_materialization": semanticRule(MappingSemanticTyped,
		EventEvidenceAcquired, EventHypothesisCreated, EventObligationCreated),
	"provider_activation_failure": semanticRule(MappingSemanticTyped,
		EventFailureRecorded),
	"provider_brain_bootstrap": semanticRule(MappingSemanticTyped,
		EventProviderProcessObserved),
	"provider_process_observation": semanticRule(MappingSemanticTyped,
		EventProviderProcessObserved),
	"reconciliation_command": semanticRule(MappingSemanticTyped,
		EventDecisionAccepted),
	"reconciliation_receipt": semanticRule(MappingSemanticTyped,
		EventEvidenceAcquired),
	"runtime_snapshot": semanticRule(MappingSemanticTyped,
		EventEvidenceAcquired, EventContextProjected),
	"transition": semanticRule(MappingSemanticTyped,
		EventWorldStarted, EventWorldTransition, EventObservationAcquired,
		EventEvidenceAcquired, EventFactAccepted),
	"working_set_event": semanticRule(MappingSemanticTyped,
		EventWorkingSetAttached, EventWorkingSetReleased, EventWorkingSetReacquired,
		EventWorkingSetRetained, EventWorkingSetTouched, EventWorkingSetInvalidated,
		EventWorkingSetScopeClosed),
	"working_set_snapshot": semanticRule(MappingSemanticTyped,
		EventWorkingSetSnapshot),
}

func SemanticSourceKinds() []string {
	result := make([]string, 0, len(frozenSemanticSourceRegistry))
	for kind := range frozenSemanticSourceRegistry {
		result = append(result, kind)
	}
	sort.Strings(result)
	return result
}

func deriveSemanticMappings(sources []SourceRecord, events []Event) ([]SourceMapping, error) {
	mappings, err := deriveMappings(sources, events, SemanticMappingSchemaV1, func(kind string) (MappingDisposition, error) {
		rule, registered := frozenSemanticSourceRegistry[kind]
		if !registered {
			return "", fmt.Errorf("semantic replay source kind %q is not frozen", kind)
		}
		return rule.disposition, nil
	})
	if err != nil {
		return nil, err
	}
	for _, mapping := range mappings {
		rule := frozenSemanticSourceRegistry[mapping.SourceKind]
		for _, event := range mapping.EventKinds {
			if _, allowed := rule.events[event]; !allowed {
				return nil, fmt.Errorf(
					"semantic replay source kind %q cannot derive event %q",
					mapping.SourceKind, event,
				)
			}
		}
	}
	return mappings, nil
}
