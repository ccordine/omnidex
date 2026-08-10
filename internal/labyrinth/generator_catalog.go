package labyrinth

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

var v1MacroKinds = []cognition.ActionKind{
	"observe", "search", "read", "navigate", "take", "use", "write",
}

const (
	stageKind  EntityKind = "stage"
	recordKind EntityKind = "record"
	fromArg               = cognition.ActionArgumentName("from")
	toArg                 = cognition.ActionArgumentName("to")
)

func buildActionCatalog(
	version string,
	plan causalPlan,
	contract causalContract,
) (cognition.ActionCatalog, []ActionDefinition, error) {
	schemas := make([]cognition.ActionSchema, len(v1MacroKinds))
	actions := make([]ActionDefinition, len(v1MacroKinds))
	for index, kind := range v1MacroKinds {
		evidencePolicy := cognition.EvidenceOptional
		if kind == contract.consumerKind(plan) {
			evidencePolicy = cognition.EvidenceRequired
		}
		schema, err := cognition.NewActionSchema(
			cognition.ActionSchemaID("labyrinth.action."+string(kind)+".v1"),
			version, kind, actionParameters(kind, plan, contract), evidencePolicy,
		)
		if err != nil {
			return cognition.ActionCatalog{}, nil, fmt.Errorf("%w: build %s schema: %v", ErrGeneration, kind, err)
		}
		schemas[index] = schema
		actions[index] = actionForSchema(schema, plan, contract)
	}
	catalog, err := cognition.NewActionCatalog("labyrinth.actions.v1", version, schemas)
	if err != nil {
		return cognition.ActionCatalog{}, nil, fmt.Errorf("%w: build action catalog: %v", ErrGeneration, err)
	}
	return catalog, actions, nil
}

func actionParameters(
	kind cognition.ActionKind,
	plan causalPlan,
	contract causalContract,
) []cognition.ActionParameterSpec {
	parameter := func(name cognition.ActionArgumentName) cognition.ActionParameterSpec {
		return cognition.ActionParameterSpec{Name: name, Required: true, MaxBytes: cognition.MaxActionValueBytes}
	}
	var parameters []cognition.ActionParameterSpec
	switch kind {
	case "observe":
		parameters = []cognition.ActionParameterSpec{}
	case "search":
		parameters = []cognition.ActionParameterSpec{parameter(queryArg)}
	case "read":
		parameters = []cognition.ActionParameterSpec{parameter(artifactArg)}
	case "navigate":
		parameters = []cognition.ActionParameterSpec{parameter(fromArg), parameter(toArg)}
	case "take":
		parameters = []cognition.ActionParameterSpec{parameter(objectArg)}
	case "use":
		parameters = []cognition.ActionParameterSpec{parameter(itemArg), parameter(targetArg)}
	case "write":
		parameters = []cognition.ActionParameterSpec{
			parameter(expectedSHA256Arg), parameter(mutationTargetArg), parameter(mutationValueArg),
		}
	default:
		panic("unregistered Labyrinth action kind")
	}
	if kind == contract.consumerKind(plan) {
		parameters = append(parameters, parameter(evidenceSetArg))
	}
	return parameters
}

func actionForSchema(
	schema cognition.ActionSchema,
	plan causalPlan,
	contract causalContract,
) ActionDefinition {
	action := ActionDefinition{
		Schema: schema, LiteralParameters: []LiteralParameter{},
		Preconditions: []Condition{}, Effects: []Effect{}, Cost: actionCost(schema.Kind),
	}
	if schema.Kind == "search" {
		action.LiteralParameters = []LiteralParameter{{
			Name: queryArg, SolverValues: []string{string(contract.query), string(contract.queryDecoy)},
		}}
	}
	switch schema.Kind {
	case "observe":
	case "navigate":
		addNavigationContract(&action)
	case "search":
		addSearchContract(&action, plan)
	case "read":
		addReadContract(&action)
	case "take":
		addTakeContract(&action, plan)
	case "use":
		addUseContract(&action)
	case "write":
		addWriteContract(&action, plan, contract)
	default:
		panic("unregistered Labyrinth action kind")
	}
	addEvidenceContract(&action, plan, contract)
	if schema.Kind == contract.consumerKind(plan) {
		action.Effects = append(action.Effects, Effect{
			Mode: EffectAssert, Predicate: PredicatePattern{
				Name: "state.completed", Arguments: []PatternArgument{{Entity: plan.mainStages[len(plan.mainStages)-1]}},
			},
		})
	}
	return action
}

func addNavigationContract(action *ActionDefinition) {
	from, to := PatternArgument{Parameter: fromArg}, PatternArgument{Parameter: toArg}
	action.Preconditions = append(action.Preconditions,
		Condition{Mode: ConditionPresent, Predicate: PredicatePattern{Name: "state.current", Arguments: []PatternArgument{from}}},
		Condition{Mode: ConditionPresent, Predicate: PredicatePattern{Name: "topology.edge", Arguments: []PatternArgument{from, to}}},
	)
	action.Effects = append(action.Effects,
		Effect{Mode: EffectRetract, Predicate: PredicatePattern{Name: "state.current", Arguments: []PatternArgument{from}}},
		Effect{Mode: EffectAssert, Predicate: PredicatePattern{Name: "state.current", Arguments: []PatternArgument{to}}},
		Effect{Mode: EffectRetract, Predicate: PredicatePattern{Name: "surface.marker", Arguments: []PatternArgument{from}}},
		Effect{Mode: EffectAssert, Predicate: PredicatePattern{Name: "surface.marker", Arguments: []PatternArgument{to}}},
	)
}

func addSearchContract(action *ActionDefinition, plan causalPlan) {
	location := PatternArgument{Entity: plan.locationForKind("search")}
	action.Preconditions = append(action.Preconditions, Condition{
		Mode: ConditionPresent, Predicate: PredicatePattern{Name: "state.current", Arguments: []PatternArgument{location}},
	})
}

func addReadContract(action *ActionDefinition) {
	artifact := PatternArgument{Parameter: artifactArg}
	action.Preconditions = append(action.Preconditions, Condition{
		Mode: ConditionPresent, Predicate: PredicatePattern{Name: "objective.read", Arguments: []PatternArgument{artifact}},
	})
	action.Effects = append(action.Effects, Effect{
		Mode: EffectAssert, Predicate: PredicatePattern{Name: "state.read", Arguments: []PatternArgument{artifact}},
	})
}

func addTakeContract(action *ActionDefinition, plan causalPlan) {
	object := PatternArgument{Parameter: objectArg}
	location := PatternArgument{Entity: plan.locationForKind("take")}
	action.Preconditions = append(action.Preconditions,
		Condition{Mode: ConditionPresent, Predicate: PredicatePattern{Name: "state.current", Arguments: []PatternArgument{location}}},
		Condition{Mode: ConditionPresent, Predicate: PredicatePattern{Name: "record.at", Arguments: []PatternArgument{object, location}}},
		Condition{Mode: ConditionPresent, Predicate: PredicatePattern{Name: "objective.object", Arguments: []PatternArgument{object}}},
	)
	action.Effects = append(action.Effects, Effect{
		Mode: EffectAssert, Predicate: PredicatePattern{Name: "inventory.held", Arguments: []PatternArgument{object}},
	})
}

func addUseContract(action *ActionDefinition) {
	item, target := PatternArgument{Parameter: itemArg}, PatternArgument{Parameter: targetArg}
	action.Preconditions = append(action.Preconditions,
		Condition{Mode: ConditionPresent, Predicate: PredicatePattern{Name: "state.current", Arguments: []PatternArgument{target}}},
		Condition{Mode: ConditionPresent, Predicate: PredicatePattern{Name: "inventory.held", Arguments: []PatternArgument{item}}},
		Condition{Mode: ConditionPresent, Predicate: PredicatePattern{Name: "objective.use", Arguments: []PatternArgument{item, target}}},
	)
	action.Effects = append(action.Effects, Effect{
		Mode: EffectAssert, Predicate: PredicatePattern{Name: "state.used", Arguments: []PatternArgument{item, target}},
	})
}

func addWriteContract(action *ActionDefinition, plan causalPlan, contract causalContract) {
	target := PatternArgument{Parameter: mutationTargetArg}
	value := PatternArgument{Parameter: mutationValueArg}
	expected := PatternArgument{Parameter: expectedSHA256Arg}
	location := PatternArgument{Entity: plan.locationForKind("write")}
	action.Preconditions = append(action.Preconditions,
		Condition{Mode: ConditionPresent, Predicate: PredicatePattern{Name: "state.current", Arguments: []PatternArgument{location}}},
		Condition{Mode: ConditionPresent, Predicate: PredicatePattern{Name: "record.at", Arguments: []PatternArgument{target, location}}},
		Condition{Mode: ConditionPresent, Predicate: PredicatePattern{Name: "record.content_hash", Arguments: []PatternArgument{target, expected}}},
		Condition{Mode: ConditionPresent, Predicate: PredicatePattern{Name: "write.allowed", Arguments: []PatternArgument{target, expected, value}}},
	)
	if contract.suite == SuiteMutate {
		action.Preconditions = append(action.Preconditions, Condition{
			Mode: ConditionPresent, Predicate: PredicatePattern{Name: "state.read", Arguments: []PatternArgument{target}},
		})
	}
	if contract.suite == SuiteCombined {
		action.Preconditions = append(action.Preconditions, Condition{
			Mode: ConditionPresent, Predicate: PredicatePattern{Name: "state.used", Arguments: []PatternArgument{
				{Entity: contract.object}, {Entity: contract.useTarget},
			}},
		})
	}
	action.Effects = append(action.Effects,
		Effect{Mode: EffectRetract, Predicate: PredicatePattern{Name: "record.content_hash", Arguments: []PatternArgument{target, expected}}},
		Effect{Mode: EffectAssert, Predicate: PredicatePattern{Name: "record.content_hash", Arguments: []PatternArgument{target, {Entity: contract.mutationCurrent}}}},
		Effect{Mode: EffectAssert, Predicate: PredicatePattern{Name: "state.mutated", Arguments: []PatternArgument{target, value}}},
	)
}

func addEvidenceContract(action *ActionDefinition, plan causalPlan, contract causalContract) {
	if action.Schema.Kind == contract.acquisitionKind(plan) {
		if action.Schema.Kind == "read" {
			action.Preconditions = append(action.Preconditions, Condition{
				Mode: ConditionPresent, Predicate: PredicatePattern{Name: "evidence.member", Arguments: []PatternArgument{
					{Entity: contract.evidenceSet}, {Parameter: artifactArg},
				}},
			})
		} else {
			for _, record := range contract.requiredRecords {
				action.Preconditions = append(action.Preconditions, Condition{
					Mode: ConditionPresent, Predicate: PredicatePattern{Name: "evidence.member", Arguments: []PatternArgument{
						{Entity: contract.evidenceSet}, {Entity: record},
					}},
				})
			}
		}
		action.Effects = append(action.Effects, Effect{
			Mode: EffectAssert, Predicate: PredicatePattern{Name: "evidence.acquired", Arguments: []PatternArgument{{Entity: contract.evidenceSet}}},
		})
	}
	if action.Schema.Kind == contract.consumerKind(plan) {
		action.Preconditions = append(action.Preconditions, Condition{
			Mode: ConditionPresent, Predicate: PredicatePattern{Name: "evidence.acquired", Arguments: []PatternArgument{{Parameter: evidenceSetArg}}},
		})
	}
}

func actionCost(kind cognition.ActionKind) int {
	switch kind {
	case "observe", "navigate":
		return 1
	case "search", "read", "take":
		return 2
	case "use":
		return 3
	case "write":
		return 4
	default:
		panic("unregistered generated action kind")
	}
}

func generatedPredicateSchemas(contract causalContract) []PredicateSchema {
	result := []PredicateSchema{
		{Name: "topology.edge", ArgumentKinds: []EntityKind{stageKind, stageKind}, Public: true},
		{Name: "surface.marker", ArgumentKinds: []EntityKind{stageKind}, Public: true},
		{Name: "surface.query", ArgumentKinds: []EntityKind{queryKind, stageKind}, Public: true},
		{Name: "surface.focus", ArgumentKinds: []EntityKind{recordKind, stageKind}, Public: true},
		{Name: "state.current", ArgumentKinds: []EntityKind{stageKind}},
		{Name: "state.completed", ArgumentKinds: []EntityKind{stageKind}},
		{Name: "record.at", ArgumentKinds: []EntityKind{recordKind, stageKind}},
		{Name: "objective.read", ArgumentKinds: []EntityKind{recordKind}},
		{Name: "state.read", ArgumentKinds: []EntityKind{recordKind}},
		{Name: "objective.object", ArgumentKinds: []EntityKind{recordKind}},
		{Name: "inventory.held", ArgumentKinds: []EntityKind{recordKind}},
		{Name: "objective.use", ArgumentKinds: []EntityKind{recordKind, stageKind}},
		{Name: "state.used", ArgumentKinds: []EntityKind{recordKind, stageKind}},
		{Name: "evidence.member", ArgumentKinds: []EntityKind{evidenceSetKind, recordKind}},
		{Name: "evidence.acquired", ArgumentKinds: []EntityKind{evidenceSetKind}},
	}
	result = append(result,
		PredicateSchema{Name: "write.allowed", ArgumentKinds: []EntityKind{recordKind, contentHashKind, mutationValueKind}},
		PredicateSchema{Name: "record.content_hash", ArgumentKinds: []EntityKind{recordKind, contentHashKind}},
		PredicateSchema{Name: "state.mutated", ArgumentKinds: []EntityKind{recordKind, mutationValueKind}},
	)
	return result
}
