package datasource

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	MaxJoinDepth      = 8
	maxJoinSearchWork = 256
)

type JoinDirection string

const (
	JoinAlongForeignKey   JoinDirection = "along_foreign_key"
	JoinAgainstForeignKey JoinDirection = "against_foreign_key"
)

type JoinStep struct {
	FromRelationID string        `json:"from_relation_id"`
	ToRelationID   string        `json:"to_relation_id"`
	ForeignKeyID   string        `json:"foreign_key_id"`
	Direction      JoinDirection `json:"direction"`
}

type JoinPath struct {
	ID    string     `json:"id"`
	Steps []JoinStep `json:"steps"`
}

type AmbiguousJoinPathError struct {
	FromRelationID string
	ToRelationID   string
	Candidates     []JoinPath
}

func (err *AmbiguousJoinPathError) Error() string {
	return fmt.Sprintf("relations %q and %q have %d equally short foreign-key paths", err.FromRelationID, err.ToRelationID, len(err.Candidates))
}

type joinEdge struct {
	to           string
	foreignKeyID string
	direction    JoinDirection
}

func PlanJoinPath(snapshot SchemaSnapshot, fromRelationID, toRelationID string) (JoinPath, error) {
	if _, err := snapshot.Relation(fromRelationID); err != nil {
		return JoinPath{}, err
	}
	if _, err := snapshot.Relation(toRelationID); err != nil {
		return JoinPath{}, err
	}
	if fromRelationID == toRelationID {
		return finalizeJoinPath(snapshot, nil), nil
	}
	graph := buildJoinGraph(snapshot)
	type searchPath struct {
		at    string
		steps []JoinStep
		seen  map[string]struct{}
	}
	queue := []searchPath{{at: fromRelationID, seen: map[string]struct{}{fromRelationID: {}}}}
	bestDepth := map[string]int{fromRelationID: 0}
	foundDepth := -1
	found := []JoinPath{}
	work := 0
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if foundDepth >= 0 && len(current.steps) >= foundDepth {
			continue
		}
		if len(current.steps) >= MaxJoinDepth {
			continue
		}
		for _, edge := range graph[current.at] {
			work++
			if work > maxJoinSearchWork {
				return JoinPath{}, fmt.Errorf("foreign-key path search exceeded %d bounded steps", maxJoinSearchWork)
			}
			if _, cycle := current.seen[edge.to]; cycle {
				continue
			}
			steps := append(append([]JoinStep(nil), current.steps...), JoinStep{FromRelationID: current.at, ToRelationID: edge.to, ForeignKeyID: edge.foreignKeyID, Direction: edge.direction})
			depth := len(steps)
			if edge.to == toRelationID {
				if foundDepth < 0 {
					foundDepth = depth
				}
				if depth == foundDepth {
					found = append(found, finalizeJoinPath(snapshot, steps))
				}
				continue
			}
			if prior, visited := bestDepth[edge.to]; visited && depth > prior {
				continue
			}
			bestDepth[edge.to] = depth
			seen := make(map[string]struct{}, len(current.seen)+1)
			for relationID := range current.seen {
				seen[relationID] = struct{}{}
			}
			seen[edge.to] = struct{}{}
			queue = append(queue, searchPath{at: edge.to, steps: steps, seen: seen})
		}
	}
	if len(found) == 0 {
		return JoinPath{}, fmt.Errorf("no foreign-key path connects relation %q to %q within depth %d", fromRelationID, toRelationID, MaxJoinDepth)
	}
	sort.Slice(found, func(i, j int) bool { return joinPathSignature(found[i]) < joinPathSignature(found[j]) })
	if len(found) > 1 {
		return JoinPath{}, &AmbiguousJoinPathError{FromRelationID: fromRelationID, ToRelationID: toRelationID, Candidates: found}
	}
	return found[0], nil
}

func ResolveJoinPath(snapshot SchemaSnapshot, fromRelationID, toRelationID, selectedPathID string) (JoinPath, error) {
	path, err := PlanJoinPath(snapshot, fromRelationID, toRelationID)
	if err == nil {
		if selectedPathID != "" {
			return JoinPath{}, fmt.Errorf("join path selection %q is unnecessary because the path is mechanically unique", selectedPathID)
		}
		return path, nil
	}
	var ambiguous *AmbiguousJoinPathError
	if !errors.As(err, &ambiguous) {
		return JoinPath{}, err
	}
	if selectedPathID == "" {
		return JoinPath{}, ambiguous
	}
	for _, candidate := range ambiguous.Candidates {
		if candidate.ID == selectedPathID {
			return candidate, nil
		}
	}
	return JoinPath{}, fmt.Errorf("join path selection %q is not a current candidate between %q and %q", selectedPathID, fromRelationID, toRelationID)
}

func buildJoinGraph(snapshot SchemaSnapshot) map[string][]joinEdge {
	graph := map[string][]joinEdge{}
	for _, relation := range snapshot.Relations {
		for _, foreignKey := range relation.ForeignKeys {
			graph[relation.ID] = append(graph[relation.ID], joinEdge{to: foreignKey.ReferencedRelationID, foreignKeyID: foreignKey.ID, direction: JoinAlongForeignKey})
			graph[foreignKey.ReferencedRelationID] = append(graph[foreignKey.ReferencedRelationID], joinEdge{to: relation.ID, foreignKeyID: foreignKey.ID, direction: JoinAgainstForeignKey})
		}
	}
	for relationID := range graph {
		sort.Slice(graph[relationID], func(i, j int) bool {
			left, right := graph[relationID][i], graph[relationID][j]
			return left.to+"\x00"+left.foreignKeyID+"\x00"+string(left.direction) < right.to+"\x00"+right.foreignKeyID+"\x00"+string(right.direction)
		})
	}
	return graph
}

func finalizeJoinPath(snapshot SchemaSnapshot, steps []JoinStep) JoinPath {
	parts := []string{snapshot.SourceID, snapshot.Fingerprint}
	for _, step := range steps {
		parts = append(parts, step.FromRelationID, step.ToRelationID, step.ForeignKeyID, string(step.Direction))
	}
	return JoinPath{ID: opaqueSchemaID("path", parts...), Steps: steps}
}

func joinPathSignature(path JoinPath) string {
	parts := make([]string, 0, len(path.Steps)*4)
	for _, step := range path.Steps {
		parts = append(parts, step.FromRelationID, step.ToRelationID, step.ForeignKeyID, string(step.Direction))
	}
	return strings.Join(parts, "\x00")
}

func findForeignKey(snapshot SchemaSnapshot, foreignKeyID string) (SchemaRelation, SchemaForeignKey, error) {
	for _, relation := range snapshot.Relations {
		for _, foreignKey := range relation.ForeignKeys {
			if foreignKey.ID == foreignKeyID {
				return relation, foreignKey, nil
			}
		}
	}
	return SchemaRelation{}, SchemaForeignKey{}, fmt.Errorf("unknown foreign key ID %q", foreignKeyID)
}
