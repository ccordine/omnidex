package labyrinth

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
)

func groundLegalRequests(
	definition Definition,
	facts factSet,
	limit int,
) ([]cognition.ActionRequest, error) {
	entities, _, err := validateEntities(definition.entities)
	if err != nil {
		return nil, err
	}
	predicates, err := validatePredicateSchemas(definition.predicateSchemas, entityKinds(entities))
	if err != nil {
		return nil, err
	}
	orderedFacts := facts.sorted()
	requests := make(map[string]cognition.ActionRequest)
	for _, action := range definition.actions {
		bindings := []map[cognition.ActionArgumentName]EntityID{{}}
		for _, condition := range action.Preconditions {
			if condition.Mode != ConditionPresent {
				continue
			}
			bindings, err = joinPredicateBindings(bindings, condition.Predicate, orderedFacts, limit)
			if err != nil {
				return nil, err
			}
			if len(bindings) == 0 {
				break
			}
		}
		bindings, err = completeBindings(bindings, action, predicates, entities, limit)
		if err != nil {
			return nil, err
		}
		for _, binding := range bindings {
			request, requestErr := requestFromBinding(action.Schema, binding)
			if requestErr != nil {
				return nil, requestErr
			}
			requests[canonicalJSON(request)] = request
			if len(requests) > limit {
				return nil, fmt.Errorf("%w: grounded action count exceeds %d", ErrSolverLimit, limit)
			}
		}
	}
	keys := make([]string, 0, len(requests))
	for key := range requests {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]cognition.ActionRequest, len(keys))
	for index, key := range keys {
		result[index] = requests[key]
	}
	return result, nil
}

func joinPredicateBindings(
	bindings []map[cognition.ActionArgumentName]EntityID,
	pattern PredicatePattern,
	facts []cognition.Predicate,
	limit int,
) ([]map[cognition.ActionArgumentName]EntityID, error) {
	joined := make([]map[cognition.ActionArgumentName]EntityID, 0)
	for _, binding := range bindings {
		for _, fact := range facts {
			if fact.Name != pattern.Name || len(fact.Args) != len(pattern.Arguments) {
				continue
			}
			candidate, matches := unifyPattern(binding, pattern, fact)
			if matches {
				joined = append(joined, candidate)
				if len(joined) > limit {
					return nil, fmt.Errorf("%w: binding count exceeds %d", ErrSolverLimit, limit)
				}
			}
		}
	}
	return joined, nil
}

func unifyPattern(
	binding map[cognition.ActionArgumentName]EntityID,
	pattern PredicatePattern,
	fact cognition.Predicate,
) (map[cognition.ActionArgumentName]EntityID, bool) {
	candidate := cloneBinding(binding)
	for index, argument := range pattern.Arguments {
		value := EntityID(fact.Args[index])
		if argument.Entity != "" && argument.Entity != value {
			return nil, false
		}
		if argument.Parameter == "" {
			continue
		}
		if existing, exists := candidate[argument.Parameter]; exists && existing != value {
			return nil, false
		}
		candidate[argument.Parameter] = value
	}
	return candidate, true
}

func completeBindings(
	bindings []map[cognition.ActionArgumentName]EntityID,
	action ActionDefinition,
	predicates map[cognition.PredicateName]PredicateSchema,
	entities map[EntityID]Entity,
	limit int,
) ([]map[cognition.ActionArgumentName]EntityID, error) {
	kinds := parameterKinds(action, predicates)
	byKind := make(map[EntityKind][]EntityID)
	for id, entity := range entities {
		byKind[entity.Kind] = append(byKind[entity.Kind], id)
	}
	for kind := range byKind {
		sort.Slice(byKind[kind], func(left, right int) bool { return byKind[kind][left] < byKind[kind][right] })
	}
	for _, parameter := range action.Schema.Parameters {
		candidates := byKind[kinds[parameter.Name]]
		if values, literal := literalSolverValues(action, parameter.Name); literal {
			candidates = make([]EntityID, len(values))
			for index, value := range values {
				candidates[index] = EntityID(value)
			}
		}
		next := make([]map[cognition.ActionArgumentName]EntityID, 0, len(bindings))
		for _, binding := range bindings {
			if _, exists := binding[parameter.Name]; exists {
				next = append(next, binding)
				continue
			}
			for _, entity := range candidates {
				candidate := cloneBinding(binding)
				candidate[parameter.Name] = entity
				next = append(next, candidate)
				if len(next) > limit {
					return nil, fmt.Errorf("%w: binding count exceeds %d", ErrSolverLimit, limit)
				}
			}
		}
		bindings = next
	}
	return bindings, nil
}

func literalSolverValues(
	action ActionDefinition,
	name cognition.ActionArgumentName,
) ([]string, bool) {
	for _, literal := range action.LiteralParameters {
		if literal.Name == name {
			return literal.SolverValues, true
		}
	}
	return nil, false
}

func requestFromBinding(
	schema cognition.ActionSchema,
	binding map[cognition.ActionArgumentName]EntityID,
) (cognition.ActionRequest, error) {
	arguments := make([]cognition.ActionArgument, len(schema.Parameters))
	for index, parameter := range schema.Parameters {
		value, exists := binding[parameter.Name]
		if !exists {
			return cognition.ActionRequest{}, fmt.Errorf("%w: solver did not bind %q", ErrGeneration, parameter.Name)
		}
		arguments[index] = cognition.ActionArgument{Name: parameter.Name, Value: string(value)}
	}
	return cognition.NewActionRequest(schema.Kind, arguments)
}

func cloneBinding(value map[cognition.ActionArgumentName]EntityID) map[cognition.ActionArgumentName]EntityID {
	cloned := make(map[cognition.ActionArgumentName]EntityID, len(value))
	for name, entity := range value {
		cloned[name] = entity
	}
	return cloned
}
