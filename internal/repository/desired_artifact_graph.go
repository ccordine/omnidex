package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	DesiredArtifactGraphSchemaV1 = "omnidex.desired-artifact-graph.v1"
	maxDesiredArtifacts          = 8
)

// DesiredArtifactGraph records accepted repository truth. It deliberately has
// no filesystem operation, target path, filename, patch, command, or ordering
// field. Physical transitions are derived later against one exact snapshot.
type DesiredArtifactGraph struct {
	Schema     string              `json:"schema"`
	ID         string              `json:"id"`
	SnapshotID string              `json:"snapshot_id"`
	OwnerID    string              `json:"owner_id"`
	Artifacts  []DesiredGoArtifact `json:"artifacts"`
}

type DesiredGoArtifact struct {
	ID                string   `json:"id"`
	RequirementQuote  string   `json:"requirement_quote"`
	PackageArtifactID string   `json:"package_artifact_id"`
	Signature         string   `json:"signature,omitempty"`
	MustExist         bool     `json:"must_exist"`
	ExistingSymbolIDs []string `json:"existing_symbol_ids"`
}

func NewDesiredArtifactGraph(
	snapshot Snapshot,
	analysis Analysis,
	ownerID string,
	artifacts []DesiredGoArtifact,
) (DesiredArtifactGraph, error) {
	graph := DesiredArtifactGraph{
		Schema: DesiredArtifactGraphSchemaV1, SnapshotID: snapshot.ID,
		OwnerID: ownerID, Artifacts: append([]DesiredGoArtifact(nil), artifacts...),
	}
	for index := range graph.Artifacts {
		graph.Artifacts[index].ExistingSymbolIDs = append(
			[]string(nil), graph.Artifacts[index].ExistingSymbolIDs...,
		)
		sort.Strings(graph.Artifacts[index].ExistingSymbolIDs)
		graph.Artifacts[index].ID = desiredGoArtifactID(graph.OwnerID, graph.Artifacts[index])
	}
	sort.Slice(graph.Artifacts, func(left, right int) bool {
		return graph.Artifacts[left].ID < graph.Artifacts[right].ID
	})
	graph.ID = desiredArtifactGraphID(graph)
	if err := graph.Validate(snapshot, analysis); err != nil {
		return DesiredArtifactGraph{}, err
	}
	return graph, nil
}

func (graph DesiredArtifactGraph) Validate(snapshot Snapshot, analysis Analysis) error {
	if err := analysis.Validate(snapshot); err != nil {
		return fmt.Errorf("desired artifact graph analysis: %w", err)
	}
	if graph.Schema != DesiredArtifactGraphSchemaV1 || graph.SnapshotID != snapshot.ID {
		return fmt.Errorf("desired artifact graph has invalid schema or snapshot authority")
	}
	if graph.OwnerID == "" || graph.OwnerID != strings.TrimSpace(graph.OwnerID) || len(graph.OwnerID) > 256 {
		return fmt.Errorf("desired artifact graph requires one bounded code-owned objective")
	}
	if len(graph.Artifacts) == 0 || len(graph.Artifacts) > maxDesiredArtifacts {
		return fmt.Errorf("desired artifact graph requires 1-%d artifacts", maxDesiredArtifacts)
	}
	previous := ""
	seenSymbols := make(map[string]struct{})
	symbols := make(map[string]Symbol, len(analysis.Symbols))
	for _, symbol := range analysis.Symbols {
		symbols[symbol.ID] = symbol
	}
	packages := make(map[string]map[string]struct{})
	for _, artifact := range analysis.Artifacts {
		if artifact.Kind == "go_package" && artifact.FileID == "" {
			packages[artifact.ID] = make(map[string]struct{})
		}
	}
	for _, edge := range analysis.Edges {
		if edge.Kind == "contains" && packages[edge.FromID] != nil {
			packages[edge.FromID][edge.ToID] = struct{}{}
		}
	}
	for index, artifact := range graph.Artifacts {
		if artifact.ID != desiredGoArtifactID(graph.OwnerID, artifact) {
			return fmt.Errorf("desired artifact %d has invalid identity", index)
		}
		if previous != "" && artifact.ID <= previous {
			return fmt.Errorf("desired artifacts must be uniquely sorted by code-owned identity")
		}
		previous = artifact.ID
		if artifact.RequirementQuote == "" || artifact.RequirementQuote != strings.TrimSpace(artifact.RequirementQuote) || len(artifact.RequirementQuote) > 1024 {
			return fmt.Errorf("desired artifact %q requires one bounded exact requirement quote", artifact.ID)
		}
		packageSymbols, packageExists := packages[artifact.PackageArtifactID]
		if !packageExists {
			return fmt.Errorf("desired artifact %q requires one exact opaque Go package authority", artifact.ID)
		}
		if artifact.MustExist && len(artifact.ExistingSymbolIDs) == 0 && strings.TrimSpace(artifact.Signature) == "" {
			return fmt.Errorf("desired new artifact %q requires one code-owned signature", artifact.ID)
		}
		if artifact.MustExist && len(artifact.ExistingSymbolIDs) != 0 && strings.TrimSpace(artifact.Signature) != "" {
			return fmt.Errorf("desired existing artifact %q cannot also claim new-declaration authority", artifact.ID)
		}
		if !artifact.MustExist && len(artifact.ExistingSymbolIDs) == 0 {
			return fmt.Errorf("desired absent artifact %q requires exact existing declaration authority", artifact.ID)
		}
		if !artifact.MustExist && strings.TrimSpace(artifact.Signature) != "" {
			return fmt.Errorf("desired absent artifact %q cannot carry generation authority", artifact.ID)
		}
		if artifact.Signature != strings.TrimSpace(artifact.Signature) || strings.ContainsAny(artifact.Signature, "\r\n") || len(artifact.Signature) > 1024 {
			return fmt.Errorf("desired artifact %q has invalid bounded signature authority", artifact.ID)
		}
		prior := ""
		for _, symbolID := range artifact.ExistingSymbolIDs {
			if symbolID == "" || symbolID <= prior {
				return fmt.Errorf("desired artifact %q has invalid or duplicate symbol authority", artifact.ID)
			}
			if _, duplicate := seenSymbols[symbolID]; duplicate {
				return fmt.Errorf("desired graph repeats existing symbol %q", symbolID)
			}
			seenSymbols[symbolID] = struct{}{}
			if _, exists := symbols[symbolID]; !exists {
				return fmt.Errorf("desired artifact %q references unknown symbol %q", artifact.ID, symbolID)
			}
			if _, owned := packageSymbols[symbolID]; !owned {
				return fmt.Errorf("desired artifact %q symbol %q belongs to another package", artifact.ID, symbolID)
			}
			prior = symbolID
		}
	}
	if graph.ID != desiredArtifactGraphID(graph) {
		return fmt.Errorf("desired artifact graph ID does not match its exact content")
	}
	return nil
}

func desiredGoArtifactID(ownerID string, artifact DesiredGoArtifact) string {
	artifact.ID = ""
	raw, _ := json.Marshal(artifact)
	value := sha256.Sum256(append([]byte(ownerID+"\x00"), raw...))
	return "desired_artifact_" + hex.EncodeToString(value[:])
}

func desiredArtifactGraphID(graph DesiredArtifactGraph) string {
	graph.ID = ""
	raw, _ := json.Marshal(graph)
	value := sha256.Sum256(raw)
	return "desired_graph_" + hex.EncodeToString(value[:])
}
