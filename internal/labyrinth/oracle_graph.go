package labyrinth

import "fmt"

func validateCausalDAG(edges []CausalEdge) error {
	adjacency := make(map[EntityID][]EntityID)
	indegree := make(map[EntityID]int)
	previous := ""
	for _, edge := range edges {
		if !validSymbol(string(edge.From)) || !validSymbol(string(edge.To)) || edge.From == edge.To {
			return fmt.Errorf("%w: causal edge is invalid", ErrGeneration)
		}
		key := string(edge.From) + "\x00" + string(edge.To)
		if previous != "" && key <= previous {
			return fmt.Errorf("%w: causal edges must be uniquely sorted", ErrGeneration)
		}
		previous = key
		adjacency[edge.From] = append(adjacency[edge.From], edge.To)
		if _, exists := indegree[edge.From]; !exists {
			indegree[edge.From] = 0
		}
		indegree[edge.To]++
	}
	queue := make([]EntityID, 0, len(indegree))
	for entity, count := range indegree {
		if count == 0 {
			queue = append(queue, entity)
		}
	}
	visited := 0
	for len(queue) > 0 {
		current := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		visited++
		for _, next := range adjacency[current] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if visited != len(indegree) {
		return fmt.Errorf("%w: causal graph contains a cycle", ErrGeneration)
	}
	return nil
}
