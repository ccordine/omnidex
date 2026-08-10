package labyrinth

import "github.com/gryph/omnidex/internal/cognition"

func extendedV1Parameters(
	kind cognition.ActionKind,
	evidenceSet bool,
) []cognition.ActionArgumentName {
	var result []cognition.ActionArgumentName
	switch kind {
	case "observe":
		result = []cognition.ActionArgumentName{}
	case "search":
		result = []cognition.ActionArgumentName{queryArg}
	case "read":
		result = []cognition.ActionArgumentName{artifactArg}
	case "navigate":
		result = []cognition.ActionArgumentName{fromArg, toArg}
	case "take":
		result = []cognition.ActionArgumentName{objectArg}
	case "use":
		result = []cognition.ActionArgumentName{itemArg, targetArg}
		if evidenceSet {
			result = append(result, evidenceSetArg)
		}
	case "write":
		result = []cognition.ActionArgumentName{expectedSHA256Arg, mutationTargetArg, mutationValueArg}
	}
	return result
}

func extendedV1Preconditions(kind cognition.ActionKind, setup extendedV1Setup) []Condition {
	switch kind {
	case "observe":
		return []Condition{}
	case "search":
		return []Condition{condition(ConditionPresent, "state.current", fixed(setup.SearchLocation))}
	case "read":
		return extendedReadPreconditions(setup)
	case "navigate":
		return []Condition{
			condition(ConditionPresent, "state.current", parameter(fromArg)),
			condition(ConditionPresent, "topology.edge", parameter(fromArg), parameter(toArg)),
		}
	case "take":
		return []Condition{
			condition(ConditionPresent, "state.current", fixed(setup.TakeLocation)),
			condition(ConditionPresent, "record.at", parameter(objectArg), fixed(setup.TakeLocation)),
			condition(ConditionPresent, "objective.object", parameter(objectArg)),
		}
	case "use":
		return extendedUsePreconditions(setup)
	case "write":
		return []Condition{
			condition(ConditionPresent, "state.current", fixed(setup.WriteLocation)),
			condition(ConditionPresent, "record.at", parameter(mutationTargetArg), fixed(setup.WriteLocation)),
			condition(ConditionPresent, "record.content_hash", parameter(mutationTargetArg), parameter(expectedSHA256Arg)),
			condition(ConditionPresent, "write.allowed", parameter(mutationTargetArg), parameter(expectedSHA256Arg), parameter(mutationValueArg)),
		}
	default:
		panic("unregistered extended v1 action")
	}
}

func extendedReadPreconditions(setup extendedV1Setup) []Condition {
	values := []Condition{condition(ConditionPresent, "objective.read", parameter(artifactArg))}
	if setup.EvidenceSet == "" {
		return values
	}
	values = append(values,
		condition(ConditionPresent, "evidence.member", fixed(setup.EvidenceSet), parameter(artifactArg)),
	)
	for _, prerequisite := range setup.SearchEvidence {
		values = append(values,
			condition(ConditionPresent, "evidence.observed", fixed(setup.EvidenceSet), fixed(prerequisite)),
		)
	}
	return values
}

func extendedUsePreconditions(setup extendedV1Setup) []Condition {
	values := []Condition{
		condition(ConditionPresent, "state.current", parameter(targetArg)),
		condition(ConditionPresent, "inventory.held", parameter(itemArg)),
		condition(ConditionPresent, "objective.use", parameter(itemArg), parameter(targetArg)),
	}
	if setup.EvidenceSet == "" {
		return values
	}
	values = append(values,
		condition(ConditionPresent, "evidence.registered", parameter(evidenceSetArg)),
	)
	for _, member := range setup.EvidenceMembers {
		values = append(values,
			condition(ConditionPresent, "evidence.observed", parameter(evidenceSetArg), fixed(member)),
		)
	}
	return values
}

func extendedV1Effects(kind cognition.ActionKind, setup extendedV1Setup) []Effect {
	switch kind {
	case "observe":
		return []Effect{}
	case "search":
		values := make([]Effect, 0, len(setup.SearchEvidence))
		for _, member := range setup.SearchEvidence {
			values = append(values,
				effect(EffectAssert, "evidence.observed", fixed(setup.EvidenceSet), fixed(member)),
			)
		}
		return values
	case "read":
		values := []Effect{effect(EffectAssert, "state.read", parameter(artifactArg))}
		if setup.EvidenceSet != "" {
			values = append(values,
				effect(EffectAssert, "evidence.observed", fixed(setup.EvidenceSet), parameter(artifactArg)),
			)
		}
		return values
	case "navigate":
		return []Effect{
			effect(EffectRetract, "state.current", parameter(fromArg)),
			effect(EffectAssert, "state.current", parameter(toArg)),
			effect(EffectRetract, "surface.marker", parameter(fromArg)),
			effect(EffectAssert, "surface.marker", parameter(toArg)),
		}
	case "take":
		return []Effect{effect(EffectAssert, "inventory.held", parameter(objectArg))}
	case "use":
		return []Effect{effect(EffectAssert, "state.used", parameter(itemArg), parameter(targetArg))}
	case "write":
		return []Effect{
			effect(EffectRetract, "record.content_hash", parameter(mutationTargetArg), parameter(expectedSHA256Arg)),
			effect(EffectAssert, "record.content_hash", parameter(mutationTargetArg), fixed(setup.MutationCurrent)),
			effect(EffectAssert, "state.mutated", parameter(mutationTargetArg), parameter(mutationValueArg)),
		}
	default:
		panic("unregistered extended v1 action")
	}
}
