package worker

import (
	"context"
	"fmt"

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
	expected := prepared.ExpectedFiles()
	current, err := repositoryfacts.BuildGitSnapshot(
		ctx, root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("snapshot authoritative repository post: %w", err)
	}
	if err := validateExactRepositoryPostInventory(source, current, expected); err != nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf(
			"authoritative repository root does not match the exact verified post state: %w", err,
		)
	}
	return current, nil
}
