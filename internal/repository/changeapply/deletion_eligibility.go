package changeapply

import (
	"context"
	"fmt"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

type DeletionCandidateSetAuthority struct {
	ctx      context.Context
	snapshot repositoryfacts.Snapshot
	analysis repositoryfacts.Analysis
}

func NewDeletionCandidateSetAuthority(
	ctx context.Context,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
) (*DeletionCandidateSetAuthority, error) {
	if err := ValidateDeletionCandidateAuthority(ctx, snapshot, analysis); err != nil {
		return nil, err
	}
	return &DeletionCandidateSetAuthority{
		ctx: ctx, snapshot: snapshot, analysis: analysis,
	}, nil
}

// ValidateDeletionCandidate proves that one code-selected exact file may enter
// a finite deletion candidate set. It returns no path or model-facing data.
// Full desired-state authority and the same checks remain mandatory at staging.
func ValidateDeletionCandidate(
	ctx context.Context,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
	fileID string,
) error {
	authority, err := NewDeletionCandidateSetAuthority(ctx, snapshot, analysis)
	if err != nil {
		return err
	}
	return authority.Validate(fileID)
}

func (authority *DeletionCandidateSetAuthority) Validate(fileID string) error {
	if authority == nil || authority.ctx == nil {
		return fmt.Errorf("repository deletion candidate set authority is unavailable")
	}
	if err := authority.ctx.Err(); err != nil {
		return fmt.Errorf("repository deletion candidate validation: %w", err)
	}
	file, err := exactDeletionCandidateFile(authority.snapshot, fileID)
	if err != nil {
		return err
	}
	tracked, err := trackedRepositorySource(authority.ctx, authority.snapshot.Root, file.Path)
	if err != nil {
		return err
	}
	if !tracked {
		return deletionCandidateIneligible(
			file.ID, DeletionCandidateIneligibleUntracked,
			fmt.Errorf("source %q has no durable tracking authority", file.ID),
		)
	}
	ignored, err := ignoredRepositoryTarget(authority.ctx, authority.snapshot.Root, file.Path, true)
	if err != nil {
		return err
	}
	if ignored {
		return deletionCandidateIneligible(
			file.ID, DeletionCandidateIneligibleIgnored,
			fmt.Errorf("source %q is ignored", file.ID),
		)
	}
	generated, err := canonicalGeneratedDeletion(authority.snapshot.Root, file)
	if err != nil {
		return err
	}
	if generated {
		return deletionCandidateIneligible(
			file.ID, DeletionCandidateIneligibleGenerated,
			fmt.Errorf("source %q contains canonical generated-source authority", file.ID),
		)
	}
	removed := make([]string, 0)
	for _, symbol := range authority.analysis.Symbols {
		if symbol.FileID == file.ID {
			removed = append(removed, symbol.ID)
		}
	}
	return validateRemovedFileSymbols(authority.analysis, file, removed)
}

func ValidateDeletionCandidateAuthority(
	ctx context.Context,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
) error {
	if ctx == nil {
		return fmt.Errorf("repository deletion candidate validation requires a context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("repository deletion candidate validation: %w", err)
	}
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("repository deletion candidate snapshot: %w", err)
	}
	if err := analysis.Validate(snapshot); err != nil {
		return fmt.Errorf("repository deletion candidate analysis: %w", err)
	}
	if !analysis.Complete {
		return fmt.Errorf("repository deletion candidate requires complete reference analysis")
	}
	if err := verifyAuthoritativeSnapshot(ctx, snapshot.Root, snapshot.ID); err != nil {
		return fmt.Errorf("repository deletion candidate authority: %w", err)
	}
	return nil
}

func exactDeletionCandidateFile(
	snapshot repositoryfacts.Snapshot,
	fileID string,
) (repositoryfacts.File, error) {
	for _, file := range snapshot.Files {
		if file.ID != fileID {
			continue
		}
		if file.Kind != repositoryfacts.EntryRegular || file.Language != "go" ||
			file.Generated || excludedFileStatePath(file.Path) {
			return repositoryfacts.File{}, deletionCandidateIneligible(
				fileID, DeletionCandidateIneligibleUnsupported, fmt.Errorf(
					"repository deletion candidate %q is generated, protected, vendored, or unsupported",
					fileID,
				))
		}
		return file, nil
	}
	return repositoryfacts.File{}, deletionCandidateIneligible(
		fileID, DeletionCandidateIneligibleUnsupported, fmt.Errorf(
			"repository deletion candidate %q is not an exact indexed source member",
			fileID,
		),
	)
}
