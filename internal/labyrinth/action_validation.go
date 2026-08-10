package labyrinth

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
)

func validateActions(
	catalog cognition.ActionCatalog,
	actions []ActionDefinition,
	entities map[EntityID]Entity,
	predicates map[cognition.PredicateName]PredicateSchema,
) error {
	if len(actions) != len(catalog.Schemas) {
		return fmt.Errorf("%w: every catalog schema requires exactly one action definition", ErrInvalidDefinition)
	}
	seen := make(map[cognition.ActionKind]struct{}, len(actions))
	for index, action := range actions {
		if err := validateAction(action, catalog, entities, predicates); err != nil {
			return fmt.Errorf("%w: action %d: %v", ErrInvalidDefinition, index, err)
		}
		if _, duplicate := seen[action.Schema.Kind]; duplicate {
			return fmt.Errorf("%w: duplicate action kind at index %d", ErrInvalidDefinition, index)
		}
		seen[action.Schema.Kind] = struct{}{}
	}
	return nil
}

func validateAction(
	action ActionDefinition,
	catalog cognition.ActionCatalog,
	entities map[EntityID]Entity,
	predicates map[cognition.PredicateName]PredicateSchema,
) error {
	if err := action.Schema.Validate(); err != nil {
		return err
	}
	registered, exists := catalog.Schema(action.Schema.Kind)
	if !exists || registered.Ref() != action.Schema.Ref() {
		return fmt.Errorf("action schema is not registered by the catalog")
	}
	if action.Cost < 1 || action.Cost > cognition.MaxTransitionCost {
		return fmt.Errorf("action cost must be between 1 and %d", cognition.MaxTransitionCost)
	}
	if len(action.Preconditions) > MaxActionPreconditions {
		return fmt.Errorf("precondition count exceeds %d", MaxActionPreconditions)
	}
	if len(action.Effects) > MaxActionEffects {
		return fmt.Errorf("effect count exceeds %d", MaxActionEffects)
	}
	parameters := make(map[cognition.ActionArgumentName]cognition.ActionParameterSpec, len(action.Schema.Parameters))
	for _, parameter := range action.Schema.Parameters {
		if !parameter.Required {
			return fmt.Errorf("symbolic parameters must be required")
		}
		parameters[parameter.Name] = parameter
	}
	literals, err := validateLiteralParameters(action.LiteralParameters, parameters)
	if err != nil {
		return err
	}
	used := make(map[cognition.ActionArgumentName]EntityKind, len(parameters))
	conditionPredicates := make(map[string]struct{}, len(action.Preconditions))
	for index, condition := range action.Preconditions {
		if condition.Mode != ConditionPresent && condition.Mode != ConditionAbsent {
			return fmt.Errorf("precondition %d has an unregistered mode", index)
		}
		if err := validatePattern(condition.Predicate, parameters, literals, used, entities, predicates); err != nil {
			return fmt.Errorf("precondition %d: %v", index, err)
		}
		key := patternKey(condition.Predicate)
		if _, duplicate := conditionPredicates[key]; duplicate {
			return fmt.Errorf("precondition %d duplicates or contradicts another precondition", index)
		}
		conditionPredicates[key] = struct{}{}
	}
	effectPredicates := make(map[string]struct{}, len(action.Effects))
	for index, effect := range action.Effects {
		if effect.Mode != EffectAssert && effect.Mode != EffectRetract {
			return fmt.Errorf("effect %d has an unregistered mode", index)
		}
		if err := validatePattern(effect.Predicate, parameters, literals, used, entities, predicates); err != nil {
			return fmt.Errorf("effect %d: %v", index, err)
		}
		key := patternKey(effect.Predicate)
		if _, duplicate := effectPredicates[key]; duplicate {
			return fmt.Errorf("effect %d duplicates or contradicts another effect", index)
		}
		effectPredicates[key] = struct{}{}
	}
	for name := range parameters {
		_, literal := literals[name]
		if _, exists := used[name]; !exists && !literal {
			return fmt.Errorf("registered parameter %q is not bound by the action", name)
		}
	}
	return nil
}

func validateLiteralParameters(
	values []LiteralParameter,
	parameters map[cognition.ActionArgumentName]cognition.ActionParameterSpec,
) (map[cognition.ActionArgumentName][]string, error) {
	result := make(map[cognition.ActionArgumentName][]string, len(values))
	previous := cognition.ActionArgumentName("")
	for index, literal := range values {
		parameter, exists := parameters[literal.Name]
		if !exists {
			return nil, fmt.Errorf("literal parameter %d is not registered by the action schema", index)
		}
		if index > 0 && literal.Name <= previous {
			return nil, fmt.Errorf("literal parameters must be uniquely sorted by name")
		}
		if len(literal.SolverValues) == 0 || len(literal.SolverValues) > cognition.MaxActionArguments {
			return nil, fmt.Errorf("literal parameter %q has invalid solver representative count", literal.Name)
		}
		for valueIndex, value := range literal.SolverValues {
			if value == "" || len(value) > parameter.MaxBytes || !utf8.ValidString(value) ||
				strings.ContainsRune(value, 0) || strings.TrimSpace(value) != value {
				return nil, fmt.Errorf("literal parameter %q representative %d is invalid", literal.Name, valueIndex)
			}
			if valueIndex > 0 && value <= literal.SolverValues[valueIndex-1] {
				return nil, fmt.Errorf("literal parameter %q representatives must be uniquely sorted", literal.Name)
			}
		}
		result[literal.Name] = literal.SolverValues
		previous = literal.Name
	}
	return result, nil
}

func validatePattern(
	pattern PredicatePattern,
	parameters map[cognition.ActionArgumentName]cognition.ActionParameterSpec,
	literals map[cognition.ActionArgumentName][]string,
	used map[cognition.ActionArgumentName]EntityKind,
	entities map[EntityID]Entity,
	predicates map[cognition.PredicateName]PredicateSchema,
) error {
	schema, exists := predicates[pattern.Name]
	if !exists || len(pattern.Arguments) != len(schema.ArgumentKinds) {
		return fmt.Errorf("pattern does not match a registered predicate schema")
	}
	for index, argument := range pattern.Arguments {
		if (argument.Parameter == "") == (argument.Entity == "") {
			return fmt.Errorf("argument %d must name exactly one parameter or entity", index)
		}
		expectedKind := schema.ArgumentKinds[index]
		if argument.Parameter != "" {
			if _, exists := parameters[argument.Parameter]; !exists {
				return fmt.Errorf("argument %d names an unregistered parameter", index)
			}
			if _, literal := literals[argument.Parameter]; literal {
				return fmt.Errorf("argument %d binds literal parameter %q as an entity", index, argument.Parameter)
			}
			if previous, exists := used[argument.Parameter]; exists && previous != expectedKind {
				return fmt.Errorf("parameter %q is bound to incompatible entity kinds", argument.Parameter)
			}
			used[argument.Parameter] = expectedKind
			continue
		}
		entity, exists := entities[argument.Entity]
		if !exists || entity.Kind != expectedKind {
			return fmt.Errorf("argument %d names an incompatible entity", index)
		}
	}
	return nil
}
