package labyrinth

import (
	"github.com/gryph/omnidex/internal/cognition"
)

func generateBind(config ExtendedGeneratorConfig) (ExtendedCase, error) {
	builder := newExtendedBuilder(config)
	left := builder.entity("first-location", stageKind, true)
	right := builder.entity("second-location", stageKind, true)
	fragmentA := builder.entity("first-evidence", recordKind, true)
	fragmentB := builder.entity("second-evidence", recordKind, true)
	item := builder.entity("portable-object", recordKind, true)
	evidenceSet := builder.entity("evidence-authority", evidenceSetKind, true)
	query := builder.entity("query-authority", queryKind, true)
	currentHash := builder.entity("unused-current-content", contentHashKind, false)
	actions, err := installExtendedV1(builder, extendedV1Setup{
		SearchLocation: left, TakeLocation: right, WriteLocation: right,
		MutationCurrent: currentHash, QueryValues: []string{string(query)}, EvidenceSet: evidenceSet,
		EvidenceMembers: []EntityID{fragmentA, fragmentB}, SearchEvidence: []EntityID{fragmentA},
	})
	if err != nil {
		return ExtendedCase{}, err
	}
	if err := addExtendedTopology(builder, left, []extendedEdge{{left, right}}); err != nil {
		return ExtendedCase{}, err
	}
	for _, fact := range []struct {
		name cognition.PredicateName
		args []EntityID
	}{
		{"surface.query", []EntityID{query, left}},
		{"record.at", []EntityID{fragmentA, left}},
		{"record.at", []EntityID{fragmentB, right}},
		{"record.at", []EntityID{item, right}},
		{"objective.read", []EntityID{fragmentB}},
		{"objective.object", []EntityID{item}},
		{"objective.use", []EntityID{item, right}},
		{"evidence.member", []EntityID{evidenceSet, fragmentA}},
		{"evidence.member", []EntityID{evidenceSet, fragmentB}},
	} {
		if err := builder.fact(fact.name, fact.args...); err != nil {
			return ExtendedCase{}, err
		}
	}
	evidenceA, err := builder.record(fragmentA, left,
		"The first registered fragment carries query "+string(query)+" and identifies one half of the authorization.")
	if err != nil {
		return ExtendedCase{}, err
	}
	evidenceB, err := builder.record(fragmentB, right,
		"The second registered fragment supplies the remaining half of the same authorization.")
	if err != nil {
		return ExtendedCase{}, err
	}
	if _, err := builder.record(item, right, "This portable object accepts the complete evidence authority here."); err != nil {
		return ExtendedCase{}, err
	}
	steps := []struct {
		schema cognition.ActionSchema
		args   []cognition.ActionArgument
	}{
		{actions["search"], []cognition.ActionArgument{{Name: queryArg, Value: string(query)}}},
		{actions["navigate"], extendedNavigateArguments(left, right)},
		{actions["take"], []cognition.ActionArgument{argument(objectArg, item)}},
		{actions["read"], extendedReadArguments(fragmentB)},
		{actions["use"], []cognition.ActionArgument{
			argument(itemArg, item), argument(targetArg, right), argument(evidenceSetArg, evidenceSet),
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
		{Evidence: evidenceA, AcquisitionActionID: created[0].ID, RequiredByActionID: created[4].ID},
		{Evidence: evidenceB, AcquisitionActionID: created[3].ID, RequiredByActionID: created[4].ID},
	}
	if err := builder.invalidRail(
		"consume-before-second-evidence", 3, cognition.ActionFailureInvalidAction,
		"use", argument(itemArg, item), argument(targetArg, right), argument(evidenceSetArg, evidenceSet),
	); err != nil {
		return ExtendedCase{}, err
	}
	builder.omissionRail("omit-first-evidence", 0, 3, cognition.ActionFailurePreconditionFailed)
	builder.omissionRail("omit-second-evidence", 3, 4, cognition.ActionFailureInvalidAction)
	goal, err := extendedGoalAll("state.used", item, right)
	if err != nil {
		return ExtendedCase{}, err
	}
	return builder.finish(goal, "Apply the registered portable object with complete supporting evidence.", 1, 2)
}
