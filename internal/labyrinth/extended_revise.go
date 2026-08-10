package labyrinth

import (
	"github.com/gryph/omnidex/internal/cognition"
)

func generateRevise(config ExtendedGeneratorConfig) (ExtendedCase, error) {
	builder := newExtendedBuilder(config)
	start := builder.entity("initial-location", stageKind, true)
	alternate := builder.entity("alternate-location", stageKind, true)
	tentative := builder.entity("tentative-evidence", recordKind, true)
	verification := builder.entity("verification-evidence", recordKind, true)
	target := builder.entity("terminal-record", recordKind, true)
	evidenceSet := builder.entity("evidence-authority", evidenceSetKind, true)
	query := builder.entity("query-authority", queryKind, true)
	value := builder.entity("registered-value", mutationValueKind, false)
	targetContent := "The registered terminal record has not yet received its authorized value."
	expectedHash := builder.boundEntity(EntityID(textSHA256(targetContent)), contentHashKind, false)
	currentHash := builder.boundEntity(EntityID(textSHA256(string(value))), contentHashKind, false)
	actions, err := installExtendedV1(builder, extendedV1Setup{
		SearchLocation: start, TakeLocation: start, WriteLocation: alternate,
		MutationCurrent: currentHash, QueryValues: []string{string(query)}, EvidenceSet: evidenceSet,
		EvidenceMembers: []EntityID{tentative, verification}, SearchEvidence: []EntityID{tentative},
	})
	if err != nil {
		return ExtendedCase{}, err
	}
	if err := addExtendedTopology(builder, start, []extendedEdge{{start, alternate}}); err != nil {
		return ExtendedCase{}, err
	}
	builder.predicate("route.enabled", []EntityKind{stageKind}, false)
	navigate := extendedAction(builder, "navigate")
	navigate.Preconditions = append(navigate.Preconditions,
		condition(ConditionPresent, "route.enabled", parameter(toArg)),
	)
	use := extendedAction(builder, "use")
	use.Effects = append(use.Effects, effect(EffectAssert, "route.enabled", fixed(alternate)))
	take := extendedAction(builder, "take")
	take.Preconditions = append(take.Preconditions,
		condition(ConditionPresent, "state.read", fixed(verification)),
	)
	write := extendedAction(builder, "write")
	write.Preconditions = append(write.Preconditions,
		condition(ConditionPresent, "state.used", fixed(verification), fixed(start)),
	)
	for _, fact := range []struct {
		name cognition.PredicateName
		args []EntityID
	}{
		{"surface.query", []EntityID{query, start}},
		{"record.at", []EntityID{tentative, start}},
		{"record.at", []EntityID{verification, start}},
		{"record.at", []EntityID{target, alternate}},
		{"objective.read", []EntityID{verification}},
		{"objective.object", []EntityID{verification}},
		{"objective.use", []EntityID{verification, start}},
		{"evidence.member", []EntityID{evidenceSet, tentative}},
		{"evidence.member", []EntityID{evidenceSet, verification}},
		{"record.content_hash", []EntityID{target, expectedHash}},
		{"write.allowed", []EntityID{target, expectedHash, value}},
	} {
		if err := builder.fact(fact.name, fact.args...); err != nil {
			return ExtendedCase{}, err
		}
	}
	falseEvidence, err := builder.record(tentative, start,
		"Query "+string(query)+" reports a tentative claim that the other visible location is unavailable.")
	if err != nil {
		return ExtendedCase{}, err
	}
	verificationEvidence, err := builder.record(verification, start,
		"Verification rejects that report. Apply this record here, then use value "+string(value)+" at the terminal record.")
	if err != nil {
		return ExtendedCase{}, err
	}
	if _, err := builder.record(target, alternate, targetContent); err != nil {
		return ExtendedCase{}, err
	}
	steps := []struct {
		schema cognition.ActionSchema
		args   []cognition.ActionArgument
	}{
		{actions["search"], []cognition.ActionArgument{{Name: queryArg, Value: string(query)}}},
		{actions["read"], extendedReadArguments(verification)},
		{actions["take"], []cognition.ActionArgument{argument(objectArg, verification)}},
		{actions["use"], []cognition.ActionArgument{
			argument(itemArg, verification), argument(targetArg, start), argument(evidenceSetArg, evidenceSet),
		}},
		{actions["navigate"], extendedNavigateArguments(start, alternate)},
		{actions["write"], []cognition.ActionArgument{
			{Name: expectedSHA256Arg, Value: string(expectedHash)},
			argument(mutationTargetArg, target), argument(mutationValueArg, value),
		}},
	}
	created := make([]WitnessAction, len(steps))
	for index, step := range steps {
		created[index], err = builder.witnessAction(step.schema, step.args...)
		if err != nil {
			return ExtendedCase{}, err
		}
	}
	builder.uses = []EvidenceUse{
		{Evidence: falseEvidence, AcquisitionActionID: created[0].ID, RequiredByActionID: created[1].ID},
		{Evidence: falseEvidence, AcquisitionActionID: created[0].ID, RequiredByActionID: created[3].ID},
		{Evidence: verificationEvidence, AcquisitionActionID: created[1].ID, RequiredByActionID: created[3].ID},
		{Evidence: verificationEvidence, AcquisitionActionID: created[1].ID, RequiredByActionID: created[4].ID},
	}
	if err := builder.invalidRail(
		"move-before-verification", 3, cognition.ActionFailurePreconditionFailed,
		"navigate", extendedNavigateArguments(start, alternate)...,
	); err != nil {
		return ExtendedCase{}, err
	}
	builder.omissionRail("omit-tentative-evidence", 0, 1, cognition.ActionFailurePreconditionFailed)
	builder.omissionRail("omit-verification-evidence", 1, 2, cognition.ActionFailurePreconditionFailed)
	builder.omissionRail("omit-route-activation", 3, 4, cognition.ActionFailurePreconditionFailed)
	goal, err := extendedGoalAll("state.mutated", target, value)
	if err != nil {
		return ExtendedCase{}, err
	}
	return builder.finish(goal, "Place the authorized value in the registered terminal record.", 1, 3)
}
