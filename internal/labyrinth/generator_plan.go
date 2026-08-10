package labyrinth

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

type causalPlan struct {
	mainStages      []EntityID
	macroKinds      []cognition.ActionKind
	actionLocations []EntityID
	navigationTo    []EntityID
	dag             []CausalEdge
}

func newCausalPlan(config GeneratorConfig) causalPlan {
	stages := make([]EntityID, config.Difficulty.SolutionDepth+1)
	for index := range stages {
		stages[index] = EntityID(fmt.Sprintf("stage-%03d", index))
	}
	kinds := suiteMacroKinds(config.Suite, config.Difficulty.SolutionDepth)
	locations := make([]EntityID, len(kinds))
	navigationTo := make([]EntityID, len(kinds))
	stageIndex := 0
	for index, kind := range kinds {
		locations[index] = stages[stageIndex]
		if kind == "navigate" {
			stageIndex++
			navigationTo[index] = stages[stageIndex]
		}
	}
	dag := make([]CausalEdge, config.Difficulty.DependencyCount)
	for index := range dag {
		dag[index] = CausalEdge{From: stages[index], To: stages[index+1]}
	}
	return causalPlan{
		mainStages: stages, macroKinds: kinds, actionLocations: locations,
		navigationTo: navigationTo, dag: dag,
	}
}

func (plan causalPlan) firstIndex(kind cognition.ActionKind) int {
	return firstMacroIndex(plan.macroKinds, kind)
}

func (plan causalPlan) locationForKind(kind cognition.ActionKind) EntityID {
	index := plan.firstIndex(kind)
	if index < 0 {
		return plan.mainStages[0]
	}
	return plan.actionLocations[index]
}

func suiteMacroKinds(suite Suite, depth int) []cognition.ActionKind {
	result := make([]cognition.ActionKind, depth)
	switch suite {
	case SuiteRetrieve:
		result[0], result[depth-1] = "search", "take"
		fillMacroKinds(result[1:depth-1], []cognition.ActionKind{"read", "navigate", "observe"})
	case SuiteRecall:
		result[0], result[depth-1] = "read", "take"
		fillMacroKinds(result[1:depth-1], []cognition.ActionKind{"navigate", "observe"})
	case SuiteUnlock:
		result[0], result[1], result[depth-1] = "search", "take", "use"
		fillMacroKinds(result[2:depth-1], []cognition.ActionKind{"navigate", "observe"})
	case SuiteMutate:
		result[0], result[1], result[depth-1] = "search", "read", "write"
		fillMacroKinds(result[2:depth-1], []cognition.ActionKind{"navigate", "observe"})
	case SuiteCombined:
		result[0], result[1], result[2], result[depth-1] = "search", "take", "use", "write"
		fillMacroKinds(result[3:depth-1], []cognition.ActionKind{"observe", "read", "navigate"})
	}
	return result
}

func fillMacroKinds(target []cognition.ActionKind, values []cognition.ActionKind) {
	for index := range target {
		target[index] = values[index%len(values)]
	}
}

func archetypeForSuite(suite Suite) TaskArchetype {
	switch suite {
	case SuiteRetrieve:
		return ArchetypeRetrieve
	case SuiteRecall:
		return ArchetypeRecall
	case SuiteUnlock:
		return ArchetypeUnlock
	case SuiteMutate:
		return ArchetypeMutate
	case SuiteCombined:
		return ArchetypeCombined
	default:
		panic("validated suite is unregistered")
	}
}
