package worker

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/queue"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
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
	expected, err := repositoryMutationPostFiles(source, command.ChangedFiles)
	if err != nil {
		return "", err
	}
	if reflect.DeepEqual(current.Files, expected) &&
		reflect.DeepEqual(current.Exclusions, source.Exclusions) {
		return queue.RepositoryMutationPost, nil
	}
	return queue.RepositoryMutationIndeterminate, nil
}

func repositoryMutationPostFiles(
	source repositoryfacts.Snapshot,
	changed []queue.RepositoryMutationFile,
) ([]repositoryfacts.File, error) {
	expected := append([]repositoryfacts.File(nil), source.Files...)
	byID := make(map[string]int, len(expected))
	for index, file := range expected {
		byID[file.ID] = index
	}
	seen := make(map[string]struct{}, len(changed))
	for _, mutation := range changed {
		index, exists := byID[mutation.FileID]
		if !exists {
			return nil, fmt.Errorf(
				"repository mutation file %q is absent from its source snapshot",
				mutation.FileID,
			)
		}
		if _, duplicate := seen[mutation.FileID]; duplicate {
			return nil, fmt.Errorf("repository mutation file %q is duplicated", mutation.FileID)
		}
		seen[mutation.FileID] = struct{}{}
		file := expected[index]
		if file.Kind != repositoryfacts.EntryRegular || file.Path != mutation.Path ||
			file.SHA256 != mutation.SourceSHA256 || file.Size != mutation.SourceSize {
			return nil, fmt.Errorf(
				"repository mutation file %q differs from immutable source authority",
				mutation.FileID,
			)
		}
		file.SHA256 = mutation.ExpectedSHA256
		file.Size = mutation.ExpectedSize
		expected[index] = file
	}
	if len(seen) == 0 {
		return nil, fmt.Errorf("repository mutation has no changed-file authority")
	}
	return expected, nil
}
