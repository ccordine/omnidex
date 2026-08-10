package labyrinth

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
)

func canonicalActionSHA256(action cognition.RegisteredAction) (string, error) {
	request := action.Request.Clone()
	sort.Slice(request.Arguments, func(left, right int) bool {
		return request.Arguments[left].Name < request.Arguments[right].Name
	})
	evidence := append([]cognition.EvidenceRef(nil), action.EvidenceRefs...)
	sort.Slice(evidence, func(left, right int) bool {
		if evidence[left].ObservationID != evidence[right].ObservationID {
			return evidence[left].ObservationID < evidence[right].ObservationID
		}
		if evidence[left].Revision.Number != evidence[right].Revision.Number {
			return evidence[left].Revision.Number < evidence[right].Revision.Number
		}
		return evidence[left].SHA256 < evidence[right].SHA256
	})
	payload := struct {
		Schema   cognition.ActionSchemaRef `json:"schema"`
		Request  cognition.ActionRequest   `json:"request"`
		Evidence []cognition.EvidenceRef   `json:"evidence_refs"`
	}{action.Schema, request, evidence}
	digest, _, err := digestJSON(payload)
	if err != nil {
		return "", fmt.Errorf("encode canonical action request: %w", err)
	}
	return digest, nil
}

func validateActionEvidence(
	action cognition.RegisteredAction,
	observations map[cognition.ObservationID]cognition.Observation,
) error {
	for _, ref := range action.EvidenceRefs {
		observation, exists := observations[ref.ObservationID]
		if !exists || observation.EvidenceRef() != ref {
			return cognition.ErrInvalidEvidence
		}
	}
	return nil
}

func applyActionDefinition(
	definition ActionDefinition,
	action cognition.RegisteredAction,
	entities map[EntityID]Entity,
	predicates map[cognition.PredicateName]PredicateSchema,
	facts factSet,
) (factSet, bool, error) {
	bindings := make(map[cognition.ActionArgumentName]EntityID, len(action.Request.Arguments))
	for _, argument := range action.Request.Arguments {
		bindings[argument.Name] = EntityID(argument.Value)
	}
	expectedKinds := parameterKinds(definition, predicates)
	for name, kind := range expectedKinds {
		entity, exists := entities[bindings[name]]
		if !exists || entity.Kind != kind {
			return nil, false, cognition.ErrInvalidAction
		}
	}
	for _, condition := range definition.Preconditions {
		predicate, err := groundPattern(condition.Predicate, bindings)
		if err != nil {
			return nil, false, cognition.ErrInvalidAction
		}
		present := facts.contains(predicate)
		if condition.Mode == ConditionPresent && !present || condition.Mode == ConditionAbsent && present {
			return nil, false, ErrPrecondition
		}
	}
	candidate := facts.clone()
	changed := false
	for _, effect := range definition.Effects {
		predicate, err := groundPattern(effect.Predicate, bindings)
		if err != nil {
			return nil, false, cognition.ErrInvalidAction
		}
		key := predicateKey(predicate)
		_, exists := candidate[key]
		switch effect.Mode {
		case EffectAssert:
			candidate[key] = predicate
			changed = changed || !exists
		case EffectRetract:
			delete(candidate, key)
			changed = changed || exists
		default:
			return nil, false, cognition.ErrInvalidAction
		}
	}
	return candidate, changed, nil
}

func parameterKinds(
	definition ActionDefinition,
	predicates map[cognition.PredicateName]PredicateSchema,
) map[cognition.ActionArgumentName]EntityKind {
	kinds := make(map[cognition.ActionArgumentName]EntityKind, len(definition.Schema.Parameters))
	patterns := make([]PredicatePattern, 0, len(definition.Preconditions)+len(definition.Effects))
	for _, condition := range definition.Preconditions {
		patterns = append(patterns, condition.Predicate)
	}
	for _, effect := range definition.Effects {
		patterns = append(patterns, effect.Predicate)
	}
	for _, pattern := range patterns {
		schema := predicates[pattern.Name]
		for index, argument := range pattern.Arguments {
			if argument.Parameter != "" {
				kinds[argument.Parameter] = schema.ArgumentKinds[index]
			}
		}
	}
	return kinds
}

func groundPattern(
	pattern PredicatePattern,
	bindings map[cognition.ActionArgumentName]EntityID,
) (cognition.Predicate, error) {
	arguments := make([]string, len(pattern.Arguments))
	for index, argument := range pattern.Arguments {
		entity := argument.Entity
		if argument.Parameter != "" {
			var exists bool
			entity, exists = bindings[argument.Parameter]
			if !exists {
				return cognition.Predicate{}, cognition.ErrInvalidAction
			}
		}
		arguments[index] = string(entity)
	}
	return cognition.NewPredicate(pattern.Name, arguments)
}
