package labyrinth

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func buildWitness(
	plan causalPlan,
	contract causalContract,
	catalog cognition.ActionCatalog,
) ([]WitnessAction, error) {
	witness := make([]WitnessAction, len(plan.macroKinds))
	for index, kind := range plan.macroKinds {
		schema, exists := catalog.Schema(kind)
		if !exists {
			return nil, fmt.Errorf("%w: witness schema %q is absent", ErrGeneration, kind)
		}
		arguments := witnessActionArguments(kind, index, plan, contract)
		for _, parameter := range schema.Parameters {
			switch parameter.Name {
			case queryArg, artifactArg, fromArg, toArg, objectArg, itemArg, targetArg,
				mutationTargetArg, expectedSHA256Arg, mutationValueArg:
			case evidenceSetArg:
				arguments = append(arguments, cognition.ActionArgument{Name: parameter.Name, Value: string(contract.evidenceSet)})
			default:
				return nil, fmt.Errorf("%w: witness schema %q has an unbound parameter %q", ErrGeneration, kind, parameter.Name)
			}
		}
		request, err := cognition.NewActionRequest(kind, arguments)
		if err != nil {
			return nil, fmt.Errorf("%w: witness request %d: %v", ErrGeneration, index, err)
		}
		witness[index] = WitnessAction{
			ID:     cognition.ActionID(fmt.Sprintf("witness-action-%03d", index)),
			Schema: schema.Ref(), Request: request, Cost: actionCost(kind),
		}
	}
	return witness, nil
}

func witnessActionArguments(
	kind cognition.ActionKind,
	index int,
	plan causalPlan,
	contract causalContract,
) []cognition.ActionArgument {
	argument := func(name cognition.ActionArgumentName, value EntityID) cognition.ActionArgument {
		return cognition.ActionArgument{Name: name, Value: string(value)}
	}
	switch kind {
	case "observe":
		return []cognition.ActionArgument{}
	case "search":
		return []cognition.ActionArgument{argument(queryArg, contract.query)}
	case "read":
		return []cognition.ActionArgument{argument(artifactArg, contract.readArtifact)}
	case "navigate":
		return []cognition.ActionArgument{
			argument(fromArg, plan.actionLocations[index]), argument(toArg, plan.navigationTo[index]),
		}
	case "take":
		return []cognition.ActionArgument{argument(objectArg, contract.object)}
	case "use":
		return []cognition.ActionArgument{
			argument(itemArg, contract.object), argument(targetArg, contract.useTarget),
		}
	case "write":
		return []cognition.ActionArgument{
			argument(mutationTargetArg, contract.mutationTarget),
			argument(expectedSHA256Arg, contract.mutationExpected),
			argument(mutationValueArg, contract.mutationValue),
		}
	default:
		panic("unregistered witness action kind")
	}
}

func buildEvidenceUses(
	evidence []EvidenceIdentity,
	witness []WitnessAction,
	contract causalContract,
) []EvidenceUse {
	uses := make([]EvidenceUse, len(evidence))
	for index, identity := range evidence {
		uses[index] = EvidenceUse{
			Evidence:            identity,
			AcquisitionActionID: witness[contract.acquisitionIndex].ID,
			RequiredByActionID:  witness[contract.consumerIndex].ID,
		}
	}
	return uses
}

func witnessCost(witness []WitnessAction) int {
	total := 0
	for _, action := range witness {
		total += action.Cost
	}
	return total
}
