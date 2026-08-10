package labyrinth

import (
	"github.com/gryph/omnidex/internal/cognition"
)

func generateTraverse(config ExtendedGeneratorConfig) (ExtendedCase, error) {
	builder := newExtendedBuilder(config)
	start := builder.entity("origin", stageKind, true)
	branch := builder.entity("side-location", stageKind, true)
	bridge := builder.entity("forward-location", stageKind, true)
	terminal := builder.entity("terminal-location", stageKind, true)
	token := builder.entity("portable-prerequisite", recordKind, true)
	currentHash := builder.entity("unused-current-content", contentHashKind, false)
	actions, err := installExtendedV1(builder, extendedV1Setup{
		SearchLocation: start, TakeLocation: branch, WriteLocation: terminal,
		MutationCurrent: currentHash, QueryValues: []string{"registered marker"},
	})
	if err != nil {
		return ExtendedCase{}, err
	}
	if err := addExtendedTopology(builder, start, []extendedEdge{
		{start, branch}, {branch, start}, {start, bridge}, {bridge, terminal},
	}); err != nil {
		return ExtendedCase{}, err
	}
	builder.predicate("route.enabled", []EntityKind{stageKind}, false)
	for _, location := range []EntityID{start, branch, terminal} {
		if err := builder.fact("route.enabled", location); err != nil {
			return ExtendedCase{}, err
		}
	}
	navigate := extendedAction(builder, "navigate")
	navigate.Preconditions = append(navigate.Preconditions,
		condition(ConditionPresent, "route.enabled", parameter(toArg)),
	)
	use := extendedAction(builder, "use")
	use.Effects = append(use.Effects, effect(EffectAssert, "route.enabled", fixed(bridge)))
	take := extendedAction(builder, "take")
	take.Preconditions = append(take.Preconditions,
		condition(ConditionPresent, "state.read", fixed(token)),
	)
	for _, fact := range []struct {
		name cognition.PredicateName
		args []EntityID
	}{
		{"record.at", []EntityID{token, branch}},
		{"objective.read", []EntityID{token}},
		{"objective.object", []EntityID{token}},
		{"objective.use", []EntityID{token, start}},
	} {
		if err := builder.fact(fact.name, fact.args...); err != nil {
			return ExtendedCase{}, err
		}
	}
	if _, err := builder.record(token, branch,
		"This portable marker activates the unavailable forward transition when applied at the origin."); err != nil {
		return ExtendedCase{}, err
	}
	steps := []struct {
		schema cognition.ActionSchema
		args   []cognition.ActionArgument
	}{
		{actions["navigate"], extendedNavigateArguments(start, branch)},
		{actions["read"], extendedReadArguments(token)},
		{actions["take"], []cognition.ActionArgument{argument(objectArg, token)}},
		{actions["navigate"], extendedNavigateArguments(branch, start)},
		{actions["use"], []cognition.ActionArgument{argument(itemArg, token), argument(targetArg, start)}},
		{actions["navigate"], extendedNavigateArguments(start, bridge)},
		{actions["navigate"], extendedNavigateArguments(bridge, terminal)},
	}
	for _, step := range steps {
		if _, err := builder.witnessAction(step.schema, step.args...); err != nil {
			return ExtendedCase{}, err
		}
	}
	if err := builder.invalidRail(
		"omit-branch-prerequisite", 0, cognition.ActionFailurePreconditionFailed,
		"use", argument(itemArg, token), argument(targetArg, start),
	); err != nil {
		return ExtendedCase{}, err
	}
	builder.omissionRail("omit-portable-prerequisite", 2, 4, cognition.ActionFailurePreconditionFailed)
	builder.omissionRail("omit-transition-activation", 4, 5, cognition.ActionFailurePreconditionFailed)
	goal, err := extendedGoal("state.current", terminal)
	if err != nil {
		return ExtendedCase{}, err
	}
	return builder.finish(goal, "Reach the registered terminal location.", 2, 3)
}
