package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/queue"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func exactRepositoryMutationClassifier(
	root string,
	source repositoryfacts.Snapshot,
) queue.RepositoryMutationClassifier {
	return func(
		ctx context.Context,
		command queue.RepositoryMutationCommand,
	) (queue.RepositoryMutationState, error) {
		current, err := repositoryfacts.BuildGitSnapshot(
			ctx, root, repositoryfacts.SnapshotOptions{},
		)
		if err != nil {
			return "", fmt.Errorf("snapshot repository mutation authority: %w", err)
		}
		return classifyRepositoryMutationSnapshots(source, current, command)
	}
}

func classifyRepositoryMutationSnapshots(
	source repositoryfacts.Snapshot,
	current repositoryfacts.Snapshot,
	command queue.RepositoryMutationCommand,
) (queue.RepositoryMutationState, error) {
	if err := source.Validate(); err != nil {
		return "", fmt.Errorf("repository mutation source snapshot: %w", err)
	}
	if err := current.Validate(); err != nil {
		return "", fmt.Errorf("repository mutation current snapshot: %w", err)
	}
	if command.SourceSnapshotID != source.ID {
		return "", fmt.Errorf(
			"repository mutation command source %q differs from snapshot %q",
			command.SourceSnapshotID, source.ID,
		)
	}
	if current.Root != source.Root || current.RepositoryID != source.RepositoryID ||
		current.HeadCommit != source.HeadCommit {
		return queue.RepositoryMutationIndeterminate, nil
	}
	if current.ID == source.ID && current.Dirty == source.Dirty &&
		current.GitStateSHA256 == source.GitStateSHA256 {
		return queue.RepositoryMutationSource, nil
	}
	expected, err := repositoryMutationExpectedStates(source, command.ChangedFiles)
	if err != nil {
		return "", err
	}
	if err := validateExactRepositoryPostInventory(source, current, expected); err == nil {
		return queue.RepositoryMutationPost, nil
	}
	return queue.RepositoryMutationIndeterminate, nil
}

func repositoryMutationExpectedStates(
	source repositoryfacts.Snapshot,
	changed []queue.RepositoryMutationFile,
) ([]changeapply.ExpectedFileState, error) {
	byID := make(map[string]repositoryfacts.File, len(source.Files))
	for _, file := range source.Files {
		byID[file.ID] = file
	}
	seen := make(map[string]struct{}, len(changed))
	expected := make([]changeapply.ExpectedFileState, 0, len(changed))
	for _, mutation := range changed {
		if _, duplicate := seen[mutation.FileID]; duplicate {
			return nil, fmt.Errorf("repository mutation file %q is duplicated", mutation.FileID)
		}
		seen[mutation.FileID] = struct{}{}
		file, exists := byID[mutation.FileID]
		if mutation.SourcePresent != exists {
			return nil, fmt.Errorf("repository mutation file %q source presence differs from immutable authority", mutation.FileID)
		}
		if exists && (file.Kind != repositoryfacts.EntryRegular || file.Path != mutation.Path ||
			file.SHA256 != mutation.SourceSHA256 || file.Size != mutation.SourceSize ||
			file.Mode != mutation.SourceMode) {
			return nil, fmt.Errorf(
				"repository mutation file %q differs from immutable source authority",
				mutation.FileID,
			)
		}
		if !exists {
			derivedID, err := repositoryfacts.FileIDForAbsentPath(source, mutation.Path)
			if err != nil || derivedID != mutation.FileID || mutation.SourceSHA256 != "" ||
				mutation.SourceSize != 0 || mutation.SourceMode != 0 {
				return nil, fmt.Errorf("repository mutation file %q has invalid absent source authority", mutation.FileID)
			}
		}
		state := changeapply.ExpectedFileState{
			FileID: mutation.FileID, Path: mutation.Path, Present: mutation.ExpectedPresent,
			SHA256: mutation.ExpectedSHA256, Size: mutation.ExpectedSize, Mode: mutation.ExpectedMode,
		}
		if err := validateExpectedRepositoryFileState(state); err != nil {
			return nil, fmt.Errorf("repository mutation file %q post authority: %w", mutation.FileID, err)
		}
		expected = append(expected, state)
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("repository mutation has no changed-file authority")
	}
	return expected, nil
}
