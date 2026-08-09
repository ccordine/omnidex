package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/queue"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func assertExactAuthoritativeRepositoryPost(
	ctx context.Context,
	root string,
	source repositoryfacts.Snapshot,
	contractID string,
	commands []testCommand,
	prepared *verifiedRepositoryChangeStage,
) error {
	_, err := exactAuthoritativeRepositoryPostSnapshot(
		ctx, root, source, contractID, commands, prepared,
	)
	return err
}

func exactAuthoritativeRepositoryPostSnapshot(
	ctx context.Context,
	root string,
	source repositoryfacts.Snapshot,
	contractID string,
	commands []testCommand,
	prepared *verifiedRepositoryChangeStage,
) (repositoryfacts.Snapshot, error) {
	if ctx == nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("authoritative repository post proof requires a context")
	}
	if err := ctx.Err(); err != nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("authoritative repository post proof: %w", err)
	}
	if prepared == nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("authoritative repository post proof requires a verified stage")
	}
	if err := prepared.RequireAuthority(contractID, commands); err != nil {
		return repositoryfacts.Snapshot{}, err
	}
	if err := source.Validate(); err != nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("authoritative repository post source: %w", err)
	}
	if root != source.Root {
		return repositoryfacts.Snapshot{}, fmt.Errorf("authoritative repository post root differs from its source authority")
	}
	files := make(map[string]repositoryfacts.File, len(source.Files))
	for _, file := range source.Files {
		files[file.ID] = file
	}
	expected := prepared.ExpectedFiles()
	changed := make([]queue.RepositoryMutationFile, len(expected))
	for index, post := range expected {
		file, exists := files[post.FileID]
		if !exists {
			return repositoryfacts.Snapshot{}, fmt.Errorf("authoritative repository post target %q is absent from its source", post.FileID)
		}
		changed[index] = queue.RepositoryMutationFile{
			FileID: post.FileID, Path: file.Path,
			SourceSHA256: file.SHA256, SourceSize: file.Size,
			ExpectedSHA256: post.SHA256, ExpectedSize: post.Size,
		}
	}
	current, err := repositoryfacts.BuildGitSnapshot(
		ctx, root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("snapshot authoritative repository post: %w", err)
	}
	state, err := classifyRepositoryMutationSnapshots(
		source, current, queue.RepositoryMutationCommand{
			SourceSnapshotID: source.ID, ChangedFiles: changed,
		},
	)
	if err != nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("classify authoritative repository post: %w", err)
	}
	if state != queue.RepositoryMutationPost {
		return repositoryfacts.Snapshot{}, fmt.Errorf("authoritative repository root does not match the exact verified post state")
	}
	return current, nil
}
