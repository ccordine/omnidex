package changeapply

import (
	"context"
	"fmt"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func verifyAuthoritativeSnapshot(ctx context.Context, root, expectedID string) error {
	current, err := repositoryfacts.BuildGitSnapshot(ctx, root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		return fmt.Errorf("rebuild authoritative repository snapshot: %w", err)
	}
	if current.ID != expectedID {
		return fmt.Errorf(
			"authoritative repository changed and snapshot %q is stale; current snapshot is %q",
			expectedID, current.ID,
		)
	}
	return nil
}
