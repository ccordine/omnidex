package labyrinth

import (
	"context"

	"github.com/gryph/omnidex/internal/cognition"
)

type EntityID string
type EntityKind string

type Entity struct {
	ID     EntityID   `json:"id"`
	Kind   EntityKind `json:"kind"`
	Public bool       `json:"public"`
}

type PredicateSchema struct {
	Name          cognition.PredicateName `json:"name"`
	ArgumentKinds []EntityKind            `json:"argument_kinds"`
	Public        bool                    `json:"public"`
}

type PatternArgument struct {
	Parameter cognition.ActionArgumentName `json:"parameter,omitempty"`
	Entity    EntityID                     `json:"entity,omitempty"`
}

type PredicatePattern struct {
	Name      cognition.PredicateName `json:"name"`
	Arguments []PatternArgument       `json:"arguments"`
}

type ConditionMode string

const (
	ConditionPresent ConditionMode = "present"
	ConditionAbsent  ConditionMode = "absent"
)

type Condition struct {
	Mode      ConditionMode    `json:"mode"`
	Predicate PredicatePattern `json:"predicate"`
}

type EffectMode string

const (
	EffectAssert  EffectMode = "assert"
	EffectRetract EffectMode = "retract"
)

type Effect struct {
	Mode      EffectMode       `json:"mode"`
	Predicate PredicatePattern `json:"predicate"`
}

// LiteralParameter names an action argument whose runtime value is exact text,
// rather than an entity identity. SolverValues are private, finite
// representatives used only to exhaustively ground the symbolic oracle.
type LiteralParameter struct {
	Name         cognition.ActionArgumentName `json:"name"`
	SolverValues []string                     `json:"solver_values"`
}

type ActionDefinition struct {
	Schema            cognition.ActionSchema `json:"schema"`
	LiteralParameters []LiteralParameter     `json:"literal_parameters"`
	Preconditions     []Condition            `json:"preconditions"`
	Effects           []Effect               `json:"effects"`
	Cost              int                    `json:"cost"`
}

// Definition is a sealed symbolic world description. Its complete contents are
// intentionally unavailable through JSON; only its digest and public catalog
// are projected outside this benchmark host.
type Definition struct {
	catalog          cognition.ActionCatalog
	entities         []Entity
	predicateSchemas []PredicateSchema
	initialFacts     []cognition.Predicate
	actions          []ActionDefinition
	goal             cognition.GoalExpression
	sha256           string
}

// Scenario binds one sealed definition to its public cognition identity.
type Scenario struct {
	ref              cognition.ScenarioRef
	definition       Definition
	definitionSHA256 string
	descriptor       PublicDescriptor
	artifactCorpus   *artifactCorpus
}

// AttemptAuthorizer validates the current code-owned execution fence. It is
// called before replay lookup or any revision check.
type AttemptAuthorizer func(context.Context, cognition.AttemptRef) error

const (
	PublicOutcomeApplied       = "applied"
	PublicOutcomeGoalSatisfied = "goal_satisfied"
)
