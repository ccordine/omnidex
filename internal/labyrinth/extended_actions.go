package labyrinth

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
)

type extendedEdge struct{ from, to EntityID }

type extendedV1Setup struct {
	SearchLocation  EntityID
	TakeLocation    EntityID
	WriteLocation   EntityID
	MutationCurrent EntityID
	QueryValues     []string
	EvidenceSet     EntityID
	EvidenceMembers []EntityID
	SearchEvidence  []EntityID
}

func installExtendedV1(
	builder *extendedBuilder,
	setup extendedV1Setup,
) (map[cognition.ActionKind]cognition.ActionSchema, error) {
	if setup.SearchLocation == "" || setup.TakeLocation == "" || setup.WriteLocation == "" ||
		setup.MutationCurrent == "" || len(setup.QueryValues) == 0 {
		return nil, fmt.Errorf("%w: extended v1 action coordinates are incomplete", ErrGeneration)
	}
	if !validExtendedEvidenceSetup(setup) {
		return nil, fmt.Errorf("%w: extended v1 evidence coordinates are incomplete", ErrGeneration)
	}
	ensureExtendedV1Kinds(builder, setup)
	for _, schema := range generatedPredicateSchemas(causalContract{}) {
		builder.predicate(schema.Name, schema.ArgumentKinds, schema.Public)
	}
	if setup.EvidenceSet != "" {
		builder.predicate("evidence.registered", []EntityKind{evidenceSetKind}, true)
		builder.predicate("evidence.observed", []EntityKind{evidenceSetKind, recordKind}, false)
		if err := builder.fact("evidence.registered", setup.EvidenceSet); err != nil {
			return nil, err
		}
	}
	actions := make(map[cognition.ActionKind]cognition.ActionSchema, len(v1MacroKinds))
	for _, kind := range v1MacroKinds {
		parameters := extendedV1Parameters(kind, setup.EvidenceSet != "")
		evidence := cognition.EvidenceOptional
		if kind == "use" && setup.EvidenceSet != "" {
			evidence = cognition.EvidenceRequired
		}
		schema, err := builder.action(
			kind, parameters, evidence,
			extendedV1Preconditions(kind, setup), extendedV1Effects(kind, setup), actionCost(kind),
		)
		if err != nil {
			return nil, err
		}
		actions[kind] = schema
		if kind == "search" {
			values := append([]string{}, setup.QueryValues...)
			sort.Strings(values)
			builder.actions[len(builder.actions)-1].LiteralParameters = []LiteralParameter{{
				Name: queryArg, SolverValues: values,
			}}
		}
	}
	return actions, nil
}

func validExtendedEvidenceSetup(setup extendedV1Setup) bool {
	if (setup.EvidenceSet == "") != (len(setup.EvidenceMembers) == 0) ||
		len(setup.SearchEvidence) > len(setup.EvidenceMembers) {
		return false
	}
	members := make(map[EntityID]struct{}, len(setup.EvidenceMembers))
	for _, member := range setup.EvidenceMembers {
		if member == "" {
			return false
		}
		members[member] = struct{}{}
	}
	if len(members) != len(setup.EvidenceMembers) {
		return false
	}
	searched := make(map[EntityID]struct{}, len(setup.SearchEvidence))
	for _, member := range setup.SearchEvidence {
		if _, exists := members[member]; !exists {
			return false
		}
		searched[member] = struct{}{}
	}
	return len(searched) == len(setup.SearchEvidence)
}

func ensureExtendedV1Kinds(builder *extendedBuilder, setup extendedV1Setup) {
	required := []struct {
		kind   EntityKind
		label  string
		public bool
	}{
		{queryKind, "query-authority", true},
		{evidenceSetKind, "evidence-authority", false},
		{mutationValueKind, "mutation-authority", false},
		{contentHashKind, "content-authority", false},
	}
	for _, entry := range required {
		found := false
		for _, entity := range builder.entities {
			found = found || entity.Kind == entry.kind
		}
		if !found {
			builder.entity(entry.label, entry.kind, entry.public)
		}
	}
}

func addExtendedTopology(
	builder *extendedBuilder,
	start EntityID,
	edges []extendedEdge,
) error {
	if err := builder.fact("surface.marker", start); err != nil {
		return err
	}
	if err := builder.fact("state.current", start); err != nil {
		return err
	}
	for _, edge := range edges {
		if err := builder.fact("topology.edge", edge.from, edge.to); err != nil {
			return err
		}
	}
	return nil
}

func extendedNavigateArguments(from, to EntityID) []cognition.ActionArgument {
	return []cognition.ActionArgument{argument(fromArg, from), argument(toArg, to)}
}

func extendedReadArguments(record EntityID) []cognition.ActionArgument {
	return []cognition.ActionArgument{argument(artifactArg, record)}
}

func extendedGoal(name cognition.PredicateName, entity EntityID) (cognition.GoalExpression, error) {
	return extendedGoalAll(name, entity)
}

func extendedGoalAll(name cognition.PredicateName, entities ...EntityID) (cognition.GoalExpression, error) {
	args := make([]string, len(entities))
	for index, entity := range entities {
		args[index] = string(entity)
	}
	predicate, err := cognition.NewPredicate(name, args)
	if err != nil {
		return cognition.GoalExpression{}, err
	}
	goal := cognition.GoalExpression{All: []cognition.Predicate{predicate}}
	return goal, goal.Validate()
}

func extendedAction(builder *extendedBuilder, kind cognition.ActionKind) *ActionDefinition {
	for index := range builder.actions {
		if builder.actions[index].Schema.Kind == kind {
			return &builder.actions[index]
		}
	}
	panic("extended v1 action is absent")
}
