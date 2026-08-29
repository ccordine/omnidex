package worker

import (
	"context"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func buildPathFreeDeletionCandidateAuthorities(
	ctx context.Context,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
) ([]artifactCandidateAuthority, error) {
	eligibility, err := changeapply.NewDeletionCandidateSetAuthority(ctx, snapshot, analysis)
	if err != nil {
		return nil, fmt.Errorf("path-free deletion candidate authority: %w", err)
	}
	files := append([]repositoryfacts.File(nil), snapshot.Files...)
	sort.Slice(files, func(left, right int) bool {
		if files[left].Path == files[right].Path {
			return files[left].ID < files[right].ID
		}
		return files[left].Path < files[right].Path
	})
	candidates := make([]artifactCandidateAuthority, 0, len(files))
	for _, file := range files {
		if file.Kind != repositoryfacts.EntryRegular || file.Language != "go" {
			continue
		}
		if err := eligibility.Validate(file.ID); err != nil {
			if _, ineligible := changeapply.DeletionCandidateIneligibilityOf(err); ineligible {
				continue
			}
			return nil, fmt.Errorf("validate path-free deletion candidate authority: %w", err)
		}
		declarations, err := artifactCandidateDeclarations(analysis, file)
		if err != nil {
			return nil, fmt.Errorf(
				"path-free deletion candidate set is unsupported: %w", err,
			)
		}
		candidates = append(candidates, artifactCandidateAuthority{
			file: file,
			input: assemblyline.ArtifactCandidateEvidence{
				Declarations: declarations,
			},
		})
	}
	if len(candidates) < 2 || len(candidates) > 8 {
		return nil, fmt.Errorf(
			"path-free deletion requires a safe finite set of 2-8 candidates; code proved %d",
			len(candidates),
		)
	}
	for index := range candidates {
		candidates[index].input.CandidateID = fmt.Sprintf("ARTIFACT_CANDIDATE_%d", index+1)
	}
	return candidates, nil
}

func selectPathFreeDeletionAuthority(
	runtime typedWorkerRuntime,
	modelName string,
	requirementQuote string,
	identities []assemblyline.ArtifactIdentity,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
) (artifactCandidateAuthority, error) {
	authorities, err := buildPathFreeDeletionCandidateAuthorities(
		runtime.Context, snapshot, analysis,
	)
	if err != nil {
		return artifactCandidateAuthority{}, err
	}
	input := assemblyline.ArtifactCandidateSelectionInput{
		RequirementQuote: requirementQuote,
		Candidates:       make([]assemblyline.ArtifactCandidateEvidence, len(authorities)),
	}
	for index := range authorities {
		input.Candidates[index] = authorities[index].input
	}
	decision, err := selectArtifactCandidate(runtime, modelName, input, identities)
	if err != nil {
		return artifactCandidateAuthority{}, err
	}
	if decision.CandidateID == assemblyline.ArtifactCandidateSelectionNone {
		return artifactCandidateAuthority{}, fmt.Errorf(
			"artifact candidate selection returned NONE; path-free deletion is unsupported",
		)
	}
	for _, authority := range authorities {
		if authority.input.CandidateID == decision.CandidateID {
			return authority, nil
		}
	}
	return artifactCandidateAuthority{}, fmt.Errorf(
		"artifact candidate selection returned unavailable authority",
	)
}
