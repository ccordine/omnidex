package labyrinth

import "github.com/gryph/omnidex/internal/cognition"

func generateOrder(config ExtendedGeneratorConfig) (ExtendedCase, error) {
	builder := newExtendedBuilder(config)
	location := builder.entity("operation-location", stageKind, true)
	instruction := builder.entity("instruction-record", recordKind, true)
	correctItem := builder.entity("authorized-object", recordKind, true)
	decoyItem := builder.entity("alternative-object", recordKind, true)
	target := builder.entity("terminal-record", recordKind, true)
	query := builder.entity("unused-query", queryKind, true)
	value := builder.entity("authorized-value", mutationValueKind, false)
	targetContent := "This terminal record awaits one authorized value."
	expectedHash := builder.boundEntity(EntityID(textSHA256(targetContent)), contentHashKind, false)
	currentHash := builder.boundEntity(EntityID(textSHA256(string(value))), contentHashKind, false)
	actions, err := installExtendedV1(builder, extendedV1Setup{
		SearchLocation: location, TakeLocation: location, WriteLocation: location,
		MutationCurrent: currentHash, QueryValues: []string{string(query)},
	})
	if err != nil {
		return ExtendedCase{}, err
	}
	if err := addExtendedTopology(builder, location, []extendedEdge{}); err != nil {
		return ExtendedCase{}, err
	}
	builder.predicate("capacity.available", []EntityKind{stageKind}, false)
	use := extendedAction(builder, "use")
	use.Preconditions = append(use.Preconditions,
		condition(ConditionPresent, "capacity.available", parameter(targetArg)),
	)
	use.Effects = append(use.Effects,
		effect(EffectRetract, "capacity.available", parameter(targetArg)),
	)
	take := extendedAction(builder, "take")
	take.Preconditions = append(take.Preconditions,
		condition(ConditionPresent, "state.read", fixed(instruction)),
	)
	write := extendedAction(builder, "write")
	write.Preconditions = append(write.Preconditions,
		condition(ConditionPresent, "state.used", fixed(correctItem), fixed(location)),
	)
	for _, fact := range []struct {
		name cognition.PredicateName
		args []EntityID
	}{
		{"surface.focus", []EntityID{instruction, location}},
		{"record.at", []EntityID{instruction, location}},
		{"record.at", []EntityID{correctItem, location}},
		{"record.at", []EntityID{decoyItem, location}},
		{"record.at", []EntityID{target, location}},
		{"objective.read", []EntityID{instruction}},
		{"objective.object", []EntityID{correctItem}},
		{"inventory.held", []EntityID{decoyItem}},
		{"objective.use", []EntityID{correctItem, location}},
		{"objective.use", []EntityID{decoyItem, location}},
		{"capacity.available", []EntityID{location}},
		{"record.content_hash", []EntityID{target, expectedHash}},
		{"write.allowed", []EntityID{target, expectedHash, value}},
	} {
		if err := builder.fact(fact.name, fact.args...); err != nil {
			return ExtendedCase{}, err
		}
	}
	if _, err := builder.record(instruction, location,
		"The mechanism has one use. Take and apply "+string(correctItem)+" before writing value "+string(value)+"; any other use consumes the mechanism."); err != nil {
		return ExtendedCase{}, err
	}
	if _, err := builder.record(correctItem, location, "A portable registered object."); err != nil {
		return ExtendedCase{}, err
	}
	if _, err := builder.record(decoyItem, location, "A different portable object."); err != nil {
		return ExtendedCase{}, err
	}
	if _, err := builder.record(target, location, targetContent); err != nil {
		return ExtendedCase{}, err
	}
	steps := []struct {
		schema cognition.ActionSchema
		args   []cognition.ActionArgument
	}{
		{actions["read"], extendedReadArguments(instruction)},
		{actions["take"], []cognition.ActionArgument{argument(objectArg, correctItem)}},
		{actions["use"], []cognition.ActionArgument{argument(itemArg, correctItem), argument(targetArg, location)}},
		{actions["write"], []cognition.ActionArgument{
			{Name: expectedSHA256Arg, Value: string(expectedHash)},
			argument(mutationTargetArg, target), argument(mutationValueArg, value),
		}},
	}
	for _, step := range steps {
		if _, err := builder.witnessAction(step.schema, step.args...); err != nil {
			return ExtendedCase{}, err
		}
	}
	if err := builder.deadEndRail(
		"consume-one-use-capacity", 1, "use",
		argument(itemArg, decoyItem), argument(targetArg, location),
	); err != nil {
		return ExtendedCase{}, err
	}
	builder.omissionRail("omit-instruction", 0, 1, cognition.ActionFailurePreconditionFailed)
	builder.omissionRail("omit-authorized-use", 2, 3, cognition.ActionFailurePreconditionFailed)
	goal, err := extendedGoalAll("state.mutated", target, value)
	if err != nil {
		return ExtendedCase{}, err
	}
	return builder.finish(goal, "Place the authorized value in the registered terminal record.", 1, 3)
}
