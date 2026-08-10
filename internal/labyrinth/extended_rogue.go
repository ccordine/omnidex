package labyrinth

import "github.com/gryph/omnidex/internal/cognition"

func generateRogue(config ExtendedGeneratorConfig) (ExtendedCase, error) {
	builder := newExtendedBuilder(config)
	start := builder.entity("initial-location", stageKind, true)
	branch := builder.entity("side-location", stageKind, true)
	vault := builder.entity("intermediate-location", stageKind, true)
	terminal := builder.entity("terminal-location", stageKind, true)
	branchToken := builder.entity("branch-object", recordKind, true)
	tentative := builder.entity("tentative-evidence", recordKind, true)
	verification := builder.entity("verification-evidence", recordKind, true)
	decoy := builder.entity("alternative-object", recordKind, true)
	target := builder.entity("terminal-record", recordKind, true)
	evidenceSet := builder.entity("evidence-authority", evidenceSetKind, true)
	query := builder.entity("query-authority", queryKind, true)
	value := builder.entity("authorized-value", mutationValueKind, false)
	targetContent := "This terminal record awaits one authorized value."
	expectedHash := builder.boundEntity(EntityID(textSHA256(targetContent)), contentHashKind, false)
	currentHash := builder.boundEntity(EntityID(textSHA256(string(value))), contentHashKind, false)
	actions, err := installExtendedV1(builder, extendedV1Setup{
		SearchLocation: start, TakeLocation: branch, WriteLocation: terminal,
		MutationCurrent: currentHash, QueryValues: []string{string(query)}, EvidenceSet: evidenceSet,
		EvidenceMembers: []EntityID{tentative, verification}, SearchEvidence: []EntityID{tentative},
	})
	if err != nil {
		return ExtendedCase{}, err
	}
	if err := addExtendedTopology(builder, start, []extendedEdge{
		{start, branch}, {branch, start}, {start, vault}, {vault, terminal},
	}); err != nil {
		return ExtendedCase{}, err
	}
	builder.predicate("route.enabled", []EntityKind{stageKind}, false)
	builder.predicate("capacity.available", []EntityKind{stageKind}, false)
	navigate := extendedAction(builder, "navigate")
	navigate.Preconditions = append(navigate.Preconditions,
		condition(ConditionPresent, "route.enabled", parameter(toArg)),
	)
	take := extendedAction(builder, "take")
	take.Effects = append(take.Effects, effect(EffectAssert, "route.enabled", fixed(vault)))
	search := extendedAction(builder, "search")
	search.Preconditions = append(search.Preconditions,
		condition(ConditionPresent, "inventory.held", fixed(branchToken)),
	)
	use := extendedAction(builder, "use")
	use.Preconditions = append(use.Preconditions,
		condition(ConditionPresent, "capacity.available", parameter(targetArg)),
	)
	use.Effects = append(use.Effects,
		effect(EffectRetract, "capacity.available", parameter(targetArg)),
		effect(EffectAssert, "route.enabled", fixed(terminal)),
	)
	write := extendedAction(builder, "write")
	write.Preconditions = append(write.Preconditions,
		condition(ConditionPresent, "state.used", fixed(verification), fixed(vault)),
	)
	for _, fact := range rogueFacts(
		start, branch, vault, terminal, branchToken, tentative, verification, decoy,
		target, evidenceSet, query, expectedHash, value,
	) {
		if err := builder.fact(fact.name, fact.args...); err != nil {
			return ExtendedCase{}, err
		}
	}
	evidenceA, err := builder.record(tentative, start,
		"Query "+string(query)+" supports a tentative claim that the terminal transition cannot be enabled.")
	if err != nil {
		return ExtendedCase{}, err
	}
	evidenceB, err := builder.record(verification, vault,
		"Verified evidence rejects the tentative claim. Apply this record here with both evidence fragments, then write value "+string(value)+".")
	if err != nil {
		return ExtendedCase{}, err
	}
	for _, record := range []struct {
		id      EntityID
		at      EntityID
		content string
	}{
		{branchToken, branch, "Taking this portable marker enables the unavailable transition from the origin."},
		{decoy, vault, "A different portable object that consumes the one-use mechanism."},
		{target, terminal, targetContent},
	} {
		if _, err := builder.record(record.id, record.at, record.content); err != nil {
			return ExtendedCase{}, err
		}
	}
	created, err := addRogueWitness(builder, actions, rogueWitnessCoordinates{
		start: start, branch: branch, vault: vault, terminal: terminal,
		branchToken: branchToken, verification: verification, evidenceSet: evidenceSet,
		query: query, target: target, expectedHash: expectedHash, value: value,
	})
	if err != nil {
		return ExtendedCase{}, err
	}
	builder.uses = []EvidenceUse{
		{Evidence: evidenceA, AcquisitionActionID: created[3].ID, RequiredByActionID: created[4].ID},
		{Evidence: evidenceA, AcquisitionActionID: created[3].ID, RequiredByActionID: created[6].ID},
		{Evidence: evidenceB, AcquisitionActionID: created[5].ID, RequiredByActionID: created[6].ID},
		{Evidence: evidenceB, AcquisitionActionID: created[5].ID, RequiredByActionID: created[7].ID},
	}
	if err := addRogueRails(builder, branch, vault, verification, decoy, evidenceSet); err != nil {
		return ExtendedCase{}, err
	}
	builder.omissionRail("omit-branch-acquisition", 1, 3, cognition.ActionFailurePreconditionFailed)
	builder.omissionRail("omit-first-evidence", 3, 5, cognition.ActionFailurePreconditionFailed)
	builder.omissionRail("omit-second-evidence", 5, 6, cognition.ActionFailureInvalidAction)
	builder.omissionRail("omit-terminal-activation", 6, 7, cognition.ActionFailurePreconditionFailed)
	used, _ := cognition.NewPredicate("state.used", []string{string(verification), string(vault)})
	mutated, _ := cognition.NewPredicate("state.mutated", []string{string(target), string(value)})
	goal, err := cognition.NewGoalExpression([]cognition.Predicate{used, mutated}, nil, nil)
	if err != nil {
		return ExtendedCase{}, err
	}
	return builder.finish(goal, "Satisfy every registered terminal predicate.", 2, 5)
}

type rogueFact struct {
	name cognition.PredicateName
	args []EntityID
}

func rogueFacts(
	start, branch, vault, terminal, branchToken, tentative, verification, decoy,
	target, evidenceSet, query, expectedHash, value EntityID,
) []rogueFact {
	return []rogueFact{
		{"surface.query", []EntityID{query, start}},
		{"record.at", []EntityID{branchToken, branch}}, {"record.at", []EntityID{tentative, start}},
		{"record.at", []EntityID{verification, vault}}, {"record.at", []EntityID{decoy, vault}},
		{"record.at", []EntityID{target, terminal}}, {"objective.object", []EntityID{branchToken}},
		{"objective.read", []EntityID{verification}}, {"inventory.held", []EntityID{verification}},
		{"inventory.held", []EntityID{decoy}}, {"objective.use", []EntityID{verification, vault}},
		{"objective.use", []EntityID{decoy, vault}}, {"evidence.member", []EntityID{evidenceSet, tentative}},
		{"evidence.member", []EntityID{evidenceSet, verification}}, {"route.enabled", []EntityID{branch}},
		{"route.enabled", []EntityID{start}}, {"capacity.available", []EntityID{vault}},
		{"record.content_hash", []EntityID{target, expectedHash}},
		{"write.allowed", []EntityID{target, expectedHash, value}},
	}
}

type rogueWitnessCoordinates struct {
	start, branch, vault, terminal                EntityID
	branchToken, verification, evidenceSet, query EntityID
	target, expectedHash, value                   EntityID
}

func addRogueWitness(
	builder *extendedBuilder,
	actions map[cognition.ActionKind]cognition.ActionSchema,
	values rogueWitnessCoordinates,
) ([]WitnessAction, error) {
	steps := []struct {
		schema cognition.ActionSchema
		args   []cognition.ActionArgument
	}{
		{actions["navigate"], extendedNavigateArguments(values.start, values.branch)},
		{actions["take"], []cognition.ActionArgument{argument(objectArg, values.branchToken)}},
		{actions["navigate"], extendedNavigateArguments(values.branch, values.start)},
		{actions["search"], []cognition.ActionArgument{{Name: queryArg, Value: string(values.query)}}},
		{actions["navigate"], extendedNavigateArguments(values.start, values.vault)},
		{actions["read"], extendedReadArguments(values.verification)},
		{actions["use"], []cognition.ActionArgument{
			argument(itemArg, values.verification), argument(targetArg, values.vault),
			argument(evidenceSetArg, values.evidenceSet),
		}},
		{actions["navigate"], extendedNavigateArguments(values.vault, values.terminal)},
		{actions["write"], []cognition.ActionArgument{
			{Name: expectedSHA256Arg, Value: string(values.expectedHash)},
			argument(mutationTargetArg, values.target), argument(mutationValueArg, values.value),
		}},
	}
	created := make([]WitnessAction, len(steps))
	for index, step := range steps {
		var err error
		created[index], err = builder.witnessAction(step.schema, step.args...)
		if err != nil {
			return nil, err
		}
	}
	return created, nil
}

func addRogueRails(
	builder *extendedBuilder,
	branch, vault, verification, decoy, evidenceSet EntityID,
) error {
	if err := builder.invalidRail(
		"skip-local-return", 1, cognition.ActionFailurePreconditionFailed,
		"navigate", extendedNavigateArguments(branch, vault)...,
	); err != nil {
		return err
	}
	if err := builder.invalidRail(
		"consume-before-second-evidence", 5, cognition.ActionFailureInvalidAction,
		"use", argument(itemArg, verification), argument(targetArg, vault), argument(evidenceSetArg, evidenceSet),
	); err != nil {
		return err
	}
	return builder.deadEndRail(
		"consume-one-use-capacity", 6, "use",
		argument(itemArg, decoy), argument(targetArg, vault), argument(evidenceSetArg, evidenceSet),
	)
}
