package changeapply

import (
	"fmt"
	"sort"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func validateRemovedFileSymbols(
	analysis repositoryfacts.Analysis,
	file repositoryfacts.File,
	selected []string,
) error {
	if !analysis.Complete {
		return fmt.Errorf("repository deletion target %q requires complete reference analysis", file.ID)
	}
	actual, ownerByAuthority := exactFileDeclarations(analysis, file.ID)
	requested := append([]string(nil), selected...)
	sort.Strings(requested)
	if len(actual) != len(requested) {
		return deletionCandidateIneligible(
			file.ID, DeletionCandidateIneligibleDeclarationAuthority,
			fmt.Errorf("repository deletion target %q does not explicitly remove every declaration", file.ID),
		)
	}
	removed := make(map[string]struct{}, len(requested)+2)
	for index, symbolID := range requested {
		if symbolID == "" || index > 0 && symbolID == requested[index-1] || symbolID != actual[index] {
			return deletionCandidateIneligible(
				file.ID, DeletionCandidateIneligibleDeclarationAuthority,
				fmt.Errorf("repository deletion target %q has incomplete or invalid declaration authority", file.ID),
			)
		}
		removed[symbolID] = struct{}{}
	}
	for _, artifact := range analysis.Artifacts {
		ownerByAuthority[artifact.ID] = artifact.FileID
		if artifact.FileID == file.ID {
			removed[artifact.ID] = struct{}{}
		}
	}
	if err := requireRemainingGoBuildMember(analysis, file, removed); err != nil {
		return err
	}
	for _, edge := range analysis.Edges {
		if edge.Kind == "contains" || edge.Kind == "builds_from" {
			continue
		}
		if _, targetRemoved := removed[edge.ToID]; !targetRemoved {
			continue
		}
		if _, sourceRemoved := removed[edge.FromID]; sourceRemoved {
			continue
		}
		owner := ownerByAuthority[edge.FromID]
		if owner == "" {
			owner = edge.EvidenceFileID
		}
		return deletionCandidateIneligible(
			file.ID, DeletionCandidateIneligibleRemainingReference, fmt.Errorf(
				"repository deletion target %q remains referenced by exact authority %q from file %q",
				file.ID, edge.FromID, owner,
			),
		)
	}
	return nil
}

func exactFileDeclarations(
	analysis repositoryfacts.Analysis,
	fileID string,
) ([]string, map[string]string) {
	actual := make([]string, 0)
	owners := make(map[string]string, len(analysis.Symbols)+len(analysis.Artifacts))
	for _, symbol := range analysis.Symbols {
		owners[symbol.ID] = symbol.FileID
		if symbol.FileID == fileID {
			actual = append(actual, symbol.ID)
		}
	}
	sort.Strings(actual)
	return actual, owners
}

func requireRemainingGoBuildMember(
	analysis repositoryfacts.Analysis,
	file repositoryfacts.File,
	removed map[string]struct{},
) error {
	sourceArtifacts := make(map[string]struct{})
	artifacts := make(map[string]repositoryfacts.Artifact, len(analysis.Artifacts))
	for _, artifact := range analysis.Artifacts {
		artifacts[artifact.ID] = artifact
		if artifact.Kind == "go_source" && artifact.FileID == file.ID {
			sourceArtifacts[artifact.ID] = struct{}{}
		}
	}
	packages := make(map[string]struct{})
	for _, edge := range analysis.Edges {
		if edge.Kind != "builds_from" {
			continue
		}
		if _, target := sourceArtifacts[edge.ToID]; target {
			packages[edge.FromID] = struct{}{}
		}
	}
	if len(sourceArtifacts) != 1 || len(packages) != 1 {
		return deletionCandidateIneligible(
			file.ID, DeletionCandidateIneligibleBuildMembership,
			fmt.Errorf("repository deletion target %q has no exact unique Go build membership authority", file.ID),
		)
	}
	for sourceID := range sourceArtifacts {
		if artifacts[sourceID].Detail["build_class"] == "test" {
			return nil
		}
	}
	for packageID := range packages {
		for _, edge := range analysis.Edges {
			if edge.Kind == "builds_from_opaque" && edge.FromID == packageID {
				return deletionCandidateIneligible(
					file.ID, DeletionCandidateIneligibleBuildMembership, fmt.Errorf(
						"repository deletion target %q has unresolved opaque Go build dependency authority %q",
						file.ID, edge.ToID,
					),
				)
			}
		}
		remaining := 0
		for _, edge := range analysis.Edges {
			if edge.Kind != "builds_from" || edge.FromID != packageID {
				continue
			}
			member := artifacts[edge.ToID]
			if member.Kind != "go_source" || member.Detail["build_class"] != "production" {
				continue
			}
			if _, deleting := removed[edge.ToID]; deleting || member.FileID == file.ID {
				continue
			}
			remaining++
		}
		if remaining == 0 {
			return deletionCandidateIneligible(
				file.ID, DeletionCandidateIneligibleBuildMembership,
				fmt.Errorf("repository deletion target %q is the last indexed Go build member of %q", file.ID, packageID),
			)
		}
	}
	return nil
}
