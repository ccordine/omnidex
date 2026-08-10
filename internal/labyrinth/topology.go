package labyrinth

import "github.com/gryph/omnidex/internal/cognition"

func generatedTopologyConnected(scenario Scenario) bool {
	stages := make(map[EntityID]struct{})
	for _, entity := range scenario.definition.entities {
		if entity.Kind == stageKind {
			stages[entity.ID] = struct{}{}
		}
	}
	adjacency := make(map[EntityID][]EntityID)
	var start EntityID
	for _, fact := range scenario.definition.initialFacts {
		switch fact.Name {
		case cognition.PredicateName("topology.edge"):
			if len(fact.Args) == 2 {
				adjacency[EntityID(fact.Args[0])] = append(adjacency[EntityID(fact.Args[0])], EntityID(fact.Args[1]))
			}
		case cognition.PredicateName("state.current"):
			if len(fact.Args) == 1 {
				start = EntityID(fact.Args[0])
			}
		}
	}
	if start == "" {
		return false
	}
	seen := map[EntityID]struct{}{start: {}}
	queue := []EntityID{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range adjacency[current] {
			if _, exists := seen[next]; exists {
				continue
			}
			seen[next] = struct{}{}
			queue = append(queue, next)
		}
	}
	return len(seen) == len(stages)
}
