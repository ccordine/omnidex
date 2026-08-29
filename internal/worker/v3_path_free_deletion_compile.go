package worker

import (
	"context"
	"fmt"
	"sort"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func compilePathFreeDesiredArtifactDeletion(
	ctx context.Context,
	authority string,
	requirementQuote string,
	selected artifactCandidateAuthority,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
) (repositoryfacts.DesiredArtifactGraph, error) {
	if selected.file.ID == "" || selected.input.CandidateID == "" {
		return repositoryfacts.DesiredArtifactGraph{}, fmt.Errorf(
			"path-free deletion selection has no exact code-held authority",
		)
	}
	var exact repositoryfacts.File
	for _, file := range snapshot.Files {
		if file.ID == selected.file.ID {
			exact = file
			break
		}
	}
	if exact.ID == "" || exact.Path != selected.file.Path ||
		exact.SHA256 != selected.file.SHA256 || exact.Size != selected.file.Size ||
		exact.Mode != selected.file.Mode || exact.Kind != selected.file.Kind {
		return repositoryfacts.DesiredArtifactGraph{}, fmt.Errorf(
			"path-free deletion selection differs from exact indexed authority",
		)
	}
	if err := changeapply.ValidateDeletionCandidate(
		ctx, snapshot, analysis, exact.ID,
	); err != nil {
		return repositoryfacts.DesiredArtifactGraph{}, fmt.Errorf(
			"revalidate selected path-free deletion candidate: %w", err,
		)
	}
	symbolIDs := make([]string, 0)
	for _, symbol := range analysis.Symbols {
		if symbol.FileID == exact.ID {
			symbolIDs = append(symbolIDs, symbol.ID)
		}
	}
	sort.Strings(symbolIDs)
	if len(symbolIDs) == 0 {
		return repositoryfacts.DesiredArtifactGraph{}, fmt.Errorf(
			"selected path-free deletion candidate has no indexed declarations",
		)
	}
	placement, err := repositoryfacts.GoPackagePlacementForSymbols(
		snapshot, analysis, symbolIDs,
	)
	if err != nil {
		return repositoryfacts.DesiredArtifactGraph{}, err
	}
	return repositoryfacts.NewDesiredArtifactGraph(
		snapshot, analysis, desiredGraphOwner(authority),
		[]repositoryfacts.DesiredGoArtifact{{
			RequirementQuote:  requirementQuote,
			PackageArtifactID: placement.ArtifactID,
			MustExist:         false,
			ExistingSymbolIDs: symbolIDs,
		}},
	)
}
