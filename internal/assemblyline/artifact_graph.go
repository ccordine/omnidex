package assemblyline

import (
	"fmt"
	"sort"
	"strings"
)

const ArtifactGraphSchemaV1 = "omnidex.artifact-graph.v1"

// ArtifactRelationKind is a code-owned normalized relation between two
// artifacts. Adapters may emit only relations they can prove from source,
// configuration, or accepted code-owned structure.
type ArtifactRelationKind string

const (
	ArtifactRelationDependsOn  ArtifactRelationKind = "depends_on"
	ArtifactRelationProvides   ArtifactRelationKind = "provides"
	ArtifactRelationConsumes   ArtifactRelationKind = "consumes"
	ArtifactRelationCalls      ArtifactRelationKind = "calls"
	ArtifactRelationComposes   ArtifactRelationKind = "composes"
	ArtifactRelationConfigures ArtifactRelationKind = "configures"
	ArtifactRelationRoutesTo   ArtifactRelationKind = "routes_to"
	ArtifactRelationPersistsTo ArtifactRelationKind = "persists_to"
	ArtifactRelationDataSource ArtifactRelationKind = "data_source"
	ArtifactRelationVerifies   ArtifactRelationKind = "verifies"
	ArtifactRelationGenerates  ArtifactRelationKind = "generates"
)

// ArtifactGraphArtifact is one code-owned artifact identity. Path is durable
// coordinator state and must never be forwarded to a coding model.
type ArtifactGraphArtifact struct {
	ID         string             `json:"id"`
	Path       string             `json:"path"`
	AdapterID  string             `json:"adapter_id"`
	Kind       TargetArtifactKind `json:"kind"`
	Interfaces []string           `json:"interfaces"`
}

// ArtifactGraphRelation records one directed, source-backed fact. Its
// endpoint IDs are graph-local code identities, never model-chosen paths.
type ArtifactGraphRelation struct {
	From string               `json:"from"`
	To   string               `json:"to"`
	Kind ArtifactRelationKind `json:"kind"`
}

// ArtifactGraph is the task-local normalized project projection. It contains
// only facts code has derived or verified; semantic uncertainties belong in a
// separate ledger question, not in this graph.
type ArtifactGraph struct {
	Schema    string                  `json:"schema"`
	Artifacts []ArtifactGraphArtifact `json:"artifacts"`
	Relations []ArtifactGraphRelation `json:"relations"`
}

func (graph ArtifactGraph) Validate() error {
	if graph.Schema != ArtifactGraphSchemaV1 {
		return fmt.Errorf("artifact graph schema must be %q", ArtifactGraphSchemaV1)
	}
	if len(graph.Artifacts) == 0 || len(graph.Artifacts) > maxTargetTreePaths {
		return fmt.Errorf("artifact graph requires 1-%d artifacts", maxTargetTreePaths)
	}
	ids := make(map[string]struct{}, len(graph.Artifacts))
	paths := make(map[string]struct{}, len(graph.Artifacts))
	for index, artifact := range graph.Artifacts {
		if err := validateArtifactGraphArtifact(artifact); err != nil {
			return fmt.Errorf("artifact graph artifact %d: %w", index, err)
		}
		if _, duplicate := ids[artifact.ID]; duplicate {
			return fmt.Errorf("artifact graph repeats artifact ID %q", artifact.ID)
		}
		if _, duplicate := paths[artifact.Path]; duplicate {
			return fmt.Errorf("artifact graph repeats artifact path %q", artifact.Path)
		}
		ids[artifact.ID] = struct{}{}
		paths[artifact.Path] = struct{}{}
	}
	seenRelations := make(map[string]struct{}, len(graph.Relations))
	for index, relation := range graph.Relations {
		if err := validateArtifactGraphRelation(relation, ids); err != nil {
			return fmt.Errorf("artifact graph relation %d: %w", index, err)
		}
		key := string(relation.Kind) + "\x00" + relation.From + "\x00" + relation.To
		if _, duplicate := seenRelations[key]; duplicate {
			return fmt.Errorf("artifact graph repeats %s relation from %q to %q", relation.Kind, relation.From, relation.To)
		}
		seenRelations[key] = struct{}{}
	}
	return nil
}

func validateArtifactGraphArtifact(artifact ArtifactGraphArtifact) error {
	if artifact.ID == "" || artifact.ID != strings.TrimSpace(artifact.ID) || len(artifact.ID) > 128 {
		return fmt.Errorf("artifact ID is invalid")
	}
	if err := validateTargetTreePath(artifact.Path); err != nil {
		return fmt.Errorf("artifact path: %w", err)
	}
	if artifact.AdapterID == "" || artifact.AdapterID != strings.TrimSpace(artifact.AdapterID) || len(artifact.AdapterID) > 128 {
		return fmt.Errorf("artifact adapter ID is invalid")
	}
	if artifact.Kind != TargetArtifactImplementation && artifact.Kind != TargetArtifactVerification {
		return fmt.Errorf("artifact kind %q is unsupported", artifact.Kind)
	}
	if len(artifact.Interfaces) > 32 {
		return fmt.Errorf("artifact has more than 32 projected interfaces")
	}
	seen := make(map[string]struct{}, len(artifact.Interfaces))
	for _, value := range artifact.Interfaces {
		if err := validateTargetTreeText("artifact interface", value, 1024); err != nil {
			return err
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("artifact repeats interface %q", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateArtifactGraphRelation(relation ArtifactGraphRelation, ids map[string]struct{}) error {
	if relation.From == "" || relation.To == "" || relation.From == relation.To {
		return fmt.Errorf("relation endpoints must be distinct known artifacts")
	}
	if _, exists := ids[relation.From]; !exists {
		return fmt.Errorf("relation source %q is unavailable", relation.From)
	}
	if _, exists := ids[relation.To]; !exists {
		return fmt.Errorf("relation target %q is unavailable", relation.To)
	}
	switch relation.Kind {
	case ArtifactRelationDependsOn, ArtifactRelationProvides, ArtifactRelationConsumes,
		ArtifactRelationCalls, ArtifactRelationComposes, ArtifactRelationConfigures,
		ArtifactRelationRoutesTo, ArtifactRelationPersistsTo, ArtifactRelationDataSource,
		ArtifactRelationVerifies, ArtifactRelationGenerates:
		return nil
	default:
		return fmt.Errorf("relation kind %q is unsupported", relation.Kind)
	}
}

// Sorted returns a stable copy for durable evidence and deterministic tests.
func (graph ArtifactGraph) Sorted() ArtifactGraph {
	copy := ArtifactGraph{Schema: graph.Schema}
	copy.Artifacts = append([]ArtifactGraphArtifact(nil), graph.Artifacts...)
	copy.Relations = append([]ArtifactGraphRelation(nil), graph.Relations...)
	for index := range copy.Artifacts {
		copy.Artifacts[index].Interfaces = append([]string(nil), copy.Artifacts[index].Interfaces...)
		sort.Strings(copy.Artifacts[index].Interfaces)
	}
	sort.Slice(copy.Artifacts, func(left, right int) bool { return copy.Artifacts[left].ID < copy.Artifacts[right].ID })
	sort.Slice(copy.Relations, func(left, right int) bool {
		if copy.Relations[left].From != copy.Relations[right].From {
			return copy.Relations[left].From < copy.Relations[right].From
		}
		if copy.Relations[left].To != copy.Relations[right].To {
			return copy.Relations[left].To < copy.Relations[right].To
		}
		return copy.Relations[left].Kind < copy.Relations[right].Kind
	})
	return copy
}
