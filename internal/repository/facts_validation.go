package repository

import (
	"fmt"
	"strings"
)

func validateSymbols(symbols []Symbol, files map[string]File, adapter AdapterIdentity) error {
	previous := ""
	for _, symbol := range symbols {
		if previous != "" && symbol.ID <= previous {
			return fmt.Errorf("repository symbols must be uniquely sorted by ID")
		}
		previous = symbol.ID
		file, exists := files[symbol.FileID]
		if !exists || file.Kind != EntryRegular || symbol.SourceSHA256 != file.SHA256 {
			return fmt.Errorf("repository symbol %q is not bound to an exact regular file", symbol.ID)
		}
		if !validOpaqueID(symbol.ID, "symbol_") || symbol.Adapter != adapter {
			return fmt.Errorf("repository symbol %q has invalid identity or adapter", symbol.ID)
		}
		if strings.TrimSpace(symbol.Kind) == "" || strings.TrimSpace(symbol.Name) == "" || strings.TrimSpace(symbol.QualifiedName) == "" {
			return fmt.Errorf("repository symbol %q requires kind, name, and qualified name", symbol.ID)
		}
		if symbol.StartByte < 0 || symbol.EndByte < symbol.StartByte || symbol.EndByte > file.Size {
			return fmt.Errorf("repository symbol %q has an invalid source span", symbol.ID)
		}
		if !validOrigin(symbol.Origin) || symbol.Confidence < 0 || symbol.Confidence > 1 {
			return fmt.Errorf("repository symbol %q has invalid provenance", symbol.ID)
		}
	}
	return nil
}

func validateArtifacts(artifacts []Artifact, files map[string]File, snapshot Snapshot, adapter AdapterIdentity) error {
	previous := ""
	for _, artifact := range artifacts {
		if previous != "" && artifact.ID <= previous {
			return fmt.Errorf("repository artifacts must be uniquely sorted by ID")
		}
		previous = artifact.ID
		if !validOpaqueID(artifact.ID, "artifact_") || artifact.Adapter != adapter {
			return fmt.Errorf("repository artifact %q has invalid identity or adapter", artifact.ID)
		}
		if artifact.FileID != "" {
			file, exists := files[artifact.FileID]
			if !exists || file.Kind != EntryRegular || artifact.SourceSHA256 != file.SHA256 {
				return fmt.Errorf("repository artifact %q references an unknown file", artifact.ID)
			}
		} else if artifact.SourceSHA256 != snapshotContentSHA256(snapshot) {
			return fmt.Errorf("repository artifact %q is not bound to the exact snapshot", artifact.ID)
		}
		if strings.TrimSpace(artifact.Kind) == "" || strings.TrimSpace(artifact.Name) == "" || !validSHA256(artifact.SourceSHA256) {
			return fmt.Errorf("repository artifact %q has invalid content", artifact.ID)
		}
		if !validOrigin(artifact.Origin) || artifact.Detail == nil {
			return fmt.Errorf("repository artifact %q has invalid provenance", artifact.ID)
		}
	}
	return nil
}

func validateEdges(
	edges []Edge,
	files map[string]File,
	endpoints map[string]struct{},
	adapter AdapterIdentity,
) error {
	previous := ""
	for _, edge := range edges {
		if previous != "" && edge.ID <= previous {
			return fmt.Errorf("repository edges must be uniquely sorted by ID")
		}
		previous = edge.ID
		if !validOpaqueID(edge.ID, "edge_") || edge.Adapter != adapter {
			return fmt.Errorf("repository edge %q has invalid identity or adapter", edge.ID)
		}
		if strings.TrimSpace(edge.FromID) == "" || strings.TrimSpace(edge.ToID) == "" || strings.TrimSpace(edge.Kind) == "" {
			return fmt.Errorf("repository edge %q requires endpoints and kind", edge.ID)
		}
		if _, exists := endpoints[edge.FromID]; !exists {
			return fmt.Errorf("repository edge %q has unknown from endpoint %q", edge.ID, edge.FromID)
		}
		if _, exists := endpoints[edge.ToID]; !exists {
			return fmt.Errorf("repository edge %q has unknown to endpoint %q", edge.ID, edge.ToID)
		}
		if edge.EvidenceFileID != "" {
			file, exists := files[edge.EvidenceFileID]
			if !exists || edge.EvidenceStartByte < 0 || edge.EvidenceEndByte < edge.EvidenceStartByte || edge.EvidenceEndByte > file.Size {
				return fmt.Errorf("repository edge %q has invalid evidence", edge.ID)
			}
		} else if edge.EvidenceStartByte != 0 || edge.EvidenceEndByte != 0 {
			return fmt.Errorf("repository edge %q has a span without an evidence file", edge.ID)
		}
		if !validOrigin(edge.Origin) || edge.Confidence < 0 || edge.Confidence > 1 {
			return fmt.Errorf("repository edge %q has invalid provenance", edge.ID)
		}
	}
	return nil
}

func validOrigin(origin FactOrigin) bool {
	switch origin {
	case OriginGoAST, OriginGoTypes, OriginGoPackages:
		return true
	default:
		return false
	}
}
