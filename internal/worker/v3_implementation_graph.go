package worker

import "github.com/gryph/omnidex/internal/artifacts"

func implementationItemIndexes(ledger artifacts.ImplementationLedgerArtifact) map[string]int {
	out := make(map[string]int, len(ledger.Items))
	for index, item := range ledger.Items {
		out[item.ID] = index
	}
	return out
}

func implementationDependencyClosure(item artifacts.ImplementationWorkItem, ledger artifacts.ImplementationLedgerArtifact, byID map[string]int) map[string]struct{} {
	out := map[string]struct{}{}
	var visit func(string)
	visit = func(id string) {
		if _, seen := out[id]; seen {
			return
		}
		index, exists := byID[id]
		if !exists {
			return
		}
		out[id] = struct{}{}
		for _, dependency := range ledger.Items[index].DependsOn {
			visit(dependency)
		}
	}
	for _, dependency := range item.DependsOn {
		visit(dependency)
	}
	return out
}

func implementationLedgerHasCycle(ledger artifacts.ImplementationLedgerArtifact, byID map[string]int) bool {
	const visiting, visited = 1, 2
	state := map[string]int{}
	var visit func(string) bool
	visit = func(id string) bool {
		if state[id] == visiting {
			return true
		}
		if state[id] == visited {
			return false
		}
		state[id] = visiting
		index, exists := byID[id]
		if exists {
			for _, dependency := range ledger.Items[index].DependsOn {
				if _, known := byID[dependency]; known && visit(dependency) {
					return true
				}
			}
		}
		state[id] = visited
		return false
	}
	for id := range byID {
		if visit(id) {
			return true
		}
	}
	return false
}
