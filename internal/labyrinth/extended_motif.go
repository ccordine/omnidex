package labyrinth

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func validateExtendedMotif(generated ExtendedCase) error {
	for index, witness := range generated.oracle.Witness {
		schema, exists := generated.public.World.Catalog.Schema(witness.Request.Kind)
		definition, defined := generated.execution.action(witness.Request.Kind)
		if !exists || !defined || schema.Ref() != witness.Schema || definition.Cost != witness.Cost {
			return fmt.Errorf("%w: extended witness action %d is not bound to execution", ErrGeneration, index)
		}
	}
	if err := validateV1SurfaceCatalog(generated.public.World.Catalog); err != nil {
		return fmt.Errorf("%w: extended catalog changed the frozen v1 surface: %v", ErrGeneration, err)
	}
	kinds := extendedWitnessKinds(generated.oracle.Witness)
	switch generated.oracle.Suite {
	case SuiteTraverse:
		if !exactKinds(kinds, "navigate", "read", "take", "navigate", "use", "navigate", "navigate") ||
			!witnessBacktracks(generated.oracle.Witness, 0, 3) ||
			!extendedScenarioActionRequires(generated.execution, "navigate", "route.enabled") ||
			!actionAsserts(generated.execution, "use", "route.enabled") ||
			!hasRail(generated.oracle, "omit-branch-prerequisite", ExtendedRailRejected) {
			return fmt.Errorf("%w: traversal motif lacks a causal branch acquisition and local return", ErrGeneration)
		}
	case SuiteBind:
		if !exactKinds(kinds, "search", "navigate", "take", "read", "use") ||
			countConsumerEvidence(generated.oracle, generated.oracle.Witness[4].ID) != 2 ||
			!hasRail(generated.oracle, "consume-before-second-evidence", ExtendedRailRejected) {
			return fmt.Errorf("%w: binding motif lacks two distant evidence acquisitions", ErrGeneration)
		}
	case SuiteRevise:
		if !exactKinds(kinds, "search", "read", "take", "use", "navigate", "write") ||
			countConsumerEvidence(generated.oracle, generated.oracle.Witness[3].ID) != 2 ||
			countConsumerEvidence(generated.oracle, generated.oracle.Witness[4].ID) != 1 ||
			!actionAsserts(generated.execution, "use", "route.enabled") ||
			!hasRail(generated.oracle, "move-before-verification", ExtendedRailRejected) {
			return fmt.Errorf("%w: revision motif lacks contradiction-gated redirection", ErrGeneration)
		}
	case SuiteOrder:
		if !exactKinds(kinds, "read", "take", "use", "write") ||
			!irreversibleCapacity(generated.execution) ||
			!hasRail(generated.oracle, "consume-one-use-capacity", ExtendedRailIrreversibleDeadEnd) {
			return fmt.Errorf("%w: ordered motif lacks a committed irreversible counterfactual", ErrGeneration)
		}
	case SuiteRogue:
		if !exactKinds(kinds,
			"navigate", "take", "navigate", "search", "navigate", "read", "use", "navigate", "write",
		) || !witnessBacktracks(generated.oracle.Witness, 0, 2) ||
			countConsumerEvidence(generated.oracle, generated.oracle.Witness[6].ID) != 2 ||
			len(generated.execution.definition.goal.All) != 2 || !irreversibleCapacity(generated.execution) ||
			!actionAsserts(generated.execution, "take", "route.enabled") ||
			!actionAsserts(generated.execution, "use", "route.enabled") ||
			!hasRail(generated.oracle, "skip-local-return", ExtendedRailRejected) ||
			!hasRail(generated.oracle, "consume-before-second-evidence", ExtendedRailRejected) ||
			!hasRail(generated.oracle, "consume-one-use-capacity", ExtendedRailIrreversibleDeadEnd) {
			return fmt.Errorf("%w: combined motif lacks independently sealed causal mechanics", ErrGeneration)
		}
	default:
		return fmt.Errorf("%w: extended suite has no registered motif", ErrGeneration)
	}
	return nil
}

func extendedWitnessKinds(values []WitnessAction) []cognition.ActionKind {
	result := make([]cognition.ActionKind, len(values))
	for index, value := range values {
		result[index] = value.Request.Kind
	}
	return result
}

func exactKinds(got []cognition.ActionKind, want ...cognition.ActionKind) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

func witnessBacktracks(values []WitnessAction, outward, back int) bool {
	if outward < 0 || back < 0 || outward >= len(values) || back >= len(values) {
		return false
	}
	return actionArgument(values[outward].Request, fromArg) == actionArgument(values[back].Request, toArg) &&
		actionArgument(values[outward].Request, toArg) == actionArgument(values[back].Request, fromArg)
}

func countConsumerEvidence(oracle ExtendedOracle, consumer cognition.ActionID) int {
	count := 0
	for _, use := range oracle.EvidenceUses {
		if use.RequiredByActionID == consumer {
			count++
		}
	}
	return count
}

func hasRail(oracle ExtendedOracle, id string, outcome ExtendedRailOutcome) bool {
	for _, rail := range oracle.InvalidRails {
		if rail.ID == id && rail.Outcome == outcome {
			return true
		}
	}
	return false
}

func extendedScenarioActionRequires(scenario Scenario, kind cognition.ActionKind, name cognition.PredicateName) bool {
	action, exists := scenario.action(kind)
	if !exists {
		return false
	}
	for _, value := range action.Preconditions {
		if value.Mode == ConditionPresent && value.Predicate.Name == name {
			return true
		}
	}
	return false
}

func actionAsserts(scenario Scenario, kind cognition.ActionKind, name cognition.PredicateName) bool {
	action, exists := scenario.action(kind)
	if !exists {
		return false
	}
	for _, value := range action.Effects {
		if value.Mode == EffectAssert && value.Predicate.Name == name {
			return true
		}
	}
	return false
}

func irreversibleCapacity(scenario Scenario) bool {
	use, useExists := scenario.action("use")
	write, writeExists := scenario.action("write")
	if !useExists || !writeExists {
		return false
	}
	return actionHasCondition(use, "capacity.available") &&
		actionHasEffect(use, EffectRetract, "capacity.available") &&
		actionHasCondition(write, "state.used")
}

func actionHasCondition(action ActionDefinition, name cognition.PredicateName) bool {
	for _, value := range action.Preconditions {
		if value.Mode == ConditionPresent && value.Predicate.Name == name {
			return true
		}
	}
	return false
}

func actionHasEffect(action ActionDefinition, mode EffectMode, name cognition.PredicateName) bool {
	for _, value := range action.Effects {
		if value.Mode == mode && value.Predicate.Name == name {
			return true
		}
	}
	return false
}
