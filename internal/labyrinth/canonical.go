package labyrinth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
)

const definitionFormat = "symbolic-world.v1"

func (definition Definition) clone() Definition {
	return Definition{
		catalog:          definition.catalog.Clone(),
		entities:         cloneEntities(definition.entities),
		predicateSchemas: clonePredicateSchemas(definition.predicateSchemas),
		initialFacts:     clonePredicates(definition.initialFacts),
		actions:          cloneActions(definition.actions),
		goal:             definition.goal.Clone(),
		sha256:           definition.sha256,
	}
}

func cloneEntities(values []Entity) []Entity {
	return append([]Entity(nil), values...)
}

func clonePredicateSchemas(values []PredicateSchema) []PredicateSchema {
	cloned := make([]PredicateSchema, len(values))
	for index, value := range values {
		value.ArgumentKinds = append([]EntityKind(nil), value.ArgumentKinds...)
		cloned[index] = value
	}
	return cloned
}

func clonePredicates(values []cognition.Predicate) []cognition.Predicate {
	cloned := make([]cognition.Predicate, len(values))
	for index, value := range values {
		cloned[index] = value.Clone()
	}
	return cloned
}

func cloneActions(values []ActionDefinition) []ActionDefinition {
	cloned := make([]ActionDefinition, len(values))
	for index, value := range values {
		value.Schema = value.Schema.Clone()
		value.LiteralParameters = cloneLiteralParameters(value.LiteralParameters)
		value.Preconditions = cloneConditions(value.Preconditions)
		value.Effects = cloneEffects(value.Effects)
		cloned[index] = value
	}
	return cloned
}

func cloneLiteralParameters(values []LiteralParameter) []LiteralParameter {
	cloned := make([]LiteralParameter, len(values))
	for index, value := range values {
		value.SolverValues = append([]string(nil), value.SolverValues...)
		cloned[index] = value
	}
	return cloned
}

func cloneConditions(values []Condition) []Condition {
	cloned := make([]Condition, len(values))
	for index, value := range values {
		value.Predicate = clonePattern(value.Predicate)
		cloned[index] = value
	}
	return cloned
}

func cloneEffects(values []Effect) []Effect {
	cloned := make([]Effect, len(values))
	for index, value := range values {
		value.Predicate = clonePattern(value.Predicate)
		cloned[index] = value
	}
	return cloned
}

func clonePattern(value PredicatePattern) PredicatePattern {
	value.Arguments = append([]PatternArgument(nil), value.Arguments...)
	return value
}

func (definition *Definition) canonicalize() {
	sort.Slice(definition.entities, func(left, right int) bool {
		return definition.entities[left].ID < definition.entities[right].ID
	})
	sort.Slice(definition.predicateSchemas, func(left, right int) bool {
		return definition.predicateSchemas[left].Name < definition.predicateSchemas[right].Name
	})
	sortPredicates(definition.initialFacts)
	canonicalizeGoal(&definition.goal)
	for index := range definition.actions {
		action := &definition.actions[index]
		for literalIndex := range action.LiteralParameters {
			sort.Strings(action.LiteralParameters[literalIndex].SolverValues)
		}
		sort.Slice(action.LiteralParameters, func(left, right int) bool {
			return action.LiteralParameters[left].Name < action.LiteralParameters[right].Name
		})
		sort.Slice(action.Preconditions, func(left, right int) bool {
			return conditionKey(action.Preconditions[left]) < conditionKey(action.Preconditions[right])
		})
		sort.Slice(action.Effects, func(left, right int) bool {
			return effectKey(action.Effects[left]) < effectKey(action.Effects[right])
		})
	}
	sort.Slice(definition.actions, func(left, right int) bool {
		if definition.actions[left].Schema.Kind != definition.actions[right].Schema.Kind {
			return definition.actions[left].Schema.Kind < definition.actions[right].Schema.Kind
		}
		return definition.actions[left].Schema.ID < definition.actions[right].Schema.ID
	})
}

func canonicalizeGoal(goal *cognition.GoalExpression) {
	for _, predicates := range [][]cognition.Predicate{goal.All, goal.Any, goal.Not} {
		sortPredicates(predicates)
	}
}

func predicateKey(predicate cognition.Predicate) string {
	return canonicalJSON(predicate)
}

func patternKey(pattern PredicatePattern) string {
	return canonicalJSON(pattern)
}

func conditionKey(condition Condition) string {
	return string(condition.Mode) + "\x00" + patternKey(condition.Predicate)
}

func effectKey(effect Effect) string {
	return string(effect.Mode) + "\x00" + patternKey(effect.Predicate)
}

func canonicalJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal validated symbolic identity: %v", err))
	}
	return string(raw)
}

func digestJSON(value any) (string, []byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", nil, err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), raw, nil
}

func (definition Definition) identity() any {
	return definition.privatePayload()
}
