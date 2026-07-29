package assemblyline

import "fmt"

type DependencyNode struct {
	ID        string
	DependsOn []string
}

// BuildDependencyWaves returns stable dependency frontiers in declaration
// order. It is language-neutral and owns all scheduling decisions.
func BuildDependencyWaves(nodes []DependencyNode) ([][]string, error) {
	ordered := make([]string, 0, len(nodes))
	remaining := make(map[string]DependencyNode, len(nodes))
	for _, node := range nodes {
		if node.ID == "" {
			return nil, fmt.Errorf("dependency node id is required")
		}
		if _, duplicate := remaining[node.ID]; duplicate {
			return nil, fmt.Errorf("dependency graph repeats node %s", node.ID)
		}
		ordered = append(ordered, node.ID)
		remaining[node.ID] = node
	}
	for _, node := range nodes {
		for _, dependency := range node.DependsOn {
			if _, exists := remaining[dependency]; !exists {
				return nil, fmt.Errorf("missing dependency %s required by node %s", dependency, node.ID)
			}
		}
	}

	completed := make(map[string]struct{}, len(nodes))
	waves := make([][]string, 0)
	for len(remaining) > 0 {
		wave := make([]string, 0)
		for _, id := range ordered {
			node, pending := remaining[id]
			if !pending {
				continue
			}
			ready := true
			for _, dependency := range node.DependsOn {
				if _, exists := completed[dependency]; !exists {
					ready = false
					break
				}
			}
			if ready {
				wave = append(wave, id)
			}
		}
		if len(wave) == 0 {
			return nil, fmt.Errorf("dependency cycle prevents completion of %d nodes", len(remaining))
		}
		for _, id := range wave {
			delete(remaining, id)
			completed[id] = struct{}{}
		}
		waves = append(waves, wave)
	}
	return waves, nil
}
