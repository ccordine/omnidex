package cognitionreplay

import (
	"fmt"
	"sort"
)

var frozenAblationSourceRegistry = map[string]semanticSourceRule{
	"ablation.root": semanticRule(MappingSemanticTyped,
		EventWorldStarted, EventGoalActivated, EventObligationCreated),
	"ablation.provider_bootstrap": semanticRule(MappingSemanticTyped,
		EventProviderProcessObserved),
	"ablation.provider_activation": semanticRule(MappingSemanticTyped,
		EventProviderProcessObserved, EventFailureRecorded),
	"ablation.transition": semanticRule(MappingSemanticTyped,
		EventWorldStarted, EventWorldTransition, EventObservationAcquired, EventEvidenceAcquired),
	"ablation.call_input": semanticRule(MappingSemanticTyped,
		EventContextProjected, EventModelCalled),
	"ablation.call_outcome": semanticRule(MappingSemanticTyped,
		EventProviderRequestDisposition, EventModelCallDisposition,
		EventDecisionAccepted, EventDecisionRejected, EventFailureRecorded),
	"ablation.model_response": semanticRule(MappingSemanticOpaque,
		EventEvidenceAcquired),
	"ablation.provider_identity": semanticRule(MappingSemanticTyped,
		EventEvidenceAcquired, EventProviderRequestDisposition),
	"ablation.provider_response_capture": semanticRule(MappingSemanticOpaque,
		EventEvidenceAcquired),
	"ablation.provider_generation": semanticRule(MappingSemanticOpaque,
		EventEvidenceAcquired),
	"ablation.call_disposition": semanticRule(MappingSemanticTyped,
		EventModelCallDisposition, EventFailureRecorded),
	"ablation.action_outcome": semanticRule(MappingSemanticTyped,
		EventActionSelected, EventWorldTransition, EventFailureRecorded),
	"ablation.ledger_event": semanticRule(MappingSemanticTyped,
		EventEvidenceAcquired),
	"ablation.working_set_initial": semanticRule(MappingSemanticTyped,
		EventWorkingSetSnapshot),
	"ablation.working_set_event": semanticRule(MappingSemanticTyped,
		EventWorkingSetAttached, EventWorkingSetReleased, EventWorkingSetReacquired,
		EventWorkingSetRetained, EventWorkingSetTouched, EventWorkingSetInvalidated,
		EventWorkingSetScopeClosed),
	"ablation.context_budget_failure": semanticRule(MappingSemanticTyped,
		EventContextProjected, EventFailureRecorded),
	"ablation.terminal": semanticRule(MappingSemanticTyped,
		EventGoalSatisfied, EventGoalFailed, EventFailureRecorded, EventEpisodeSealed),
}

func AblationSemanticSourceKinds() []string {
	result := make([]string, 0, len(frozenAblationSourceRegistry))
	for kind := range frozenAblationSourceRegistry {
		result = append(result, kind)
	}
	sort.Strings(result)
	return result
}

func deriveAblationMappings(sources []SourceRecord, events []Event) ([]SourceMapping, error) {
	mappings, err := deriveMappings(
		sources, events, AblationSemanticMappingSchemaV1,
		func(kind string) (MappingDisposition, error) {
			rule, registered := frozenAblationSourceRegistry[kind]
			if !registered {
				return "", fmt.Errorf("ablation semantic source kind %q is not frozen", kind)
			}
			return rule.disposition, nil
		},
	)
	if err != nil {
		return nil, err
	}
	for _, mapping := range mappings {
		rule := frozenAblationSourceRegistry[mapping.SourceKind]
		for _, event := range mapping.EventKinds {
			if _, allowed := rule.events[event]; !allowed {
				return nil, fmt.Errorf(
					"ablation semantic source kind %q cannot derive event %q",
					mapping.SourceKind, event,
				)
			}
		}
	}
	return mappings, nil
}

func ablationSourceRegistryDigest() string {
	type ruleDigest struct {
		Kind        string             `json:"kind"`
		Disposition MappingDisposition `json:"disposition"`
		EventKinds  []EventKind        `json:"event_kinds"`
	}
	kinds := AblationSemanticSourceKinds()
	values := make([]ruleDigest, len(kinds))
	for index, kind := range kinds {
		rule := frozenAblationSourceRegistry[kind]
		events := make([]EventKind, 0, len(rule.events))
		for event := range rule.events {
			events = append(events, event)
		}
		sort.Slice(events, func(left, right int) bool { return events[left] < events[right] })
		values[index] = ruleDigest{Kind: kind, Disposition: rule.disposition, EventKinds: events}
	}
	raw, err := marshalCanonical(values)
	if err != nil {
		panic("static ablation replay registry is not canonical: " + err.Error())
	}
	return digestBytes(raw)
}

const ablationSourceRegistrySHA256V1 = "beaea40a440a814f2354cc60fe0b9ee7b4ffc1cdac7dbb0047412ad0349161f8"
