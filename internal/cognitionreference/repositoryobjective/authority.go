package repositoryobjective

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

type projectedAuthority struct {
	authoritative repositoryfacts.Snapshot
	projected     repositoryfacts.Snapshot
	workspace     *changeapply.SnapshotWorkspace
}

func captureProjectedAuthority(ctx context.Context, root string) (projectedAuthority, error) {
	snapshot, err := repositoryfacts.BuildGitSnapshot(
		ctx, root, repositoryfacts.SnapshotOptions{MaxFiles: maxRepositoryFiles},
	)
	if err != nil {
		return projectedAuthority{}, fmt.Errorf("capture repository objective snapshot: %w", err)
	}
	workspace, err := changeapply.NewSnapshotWorkspace(ctx, snapshot)
	if err != nil {
		return projectedAuthority{}, fmt.Errorf("project exact repository objective snapshot: %w", err)
	}
	projected := snapshot
	projected.Root = workspace.Root()
	if err := projected.Validate(); err != nil {
		return projectedAuthority{}, errors.Join(
			fmt.Errorf("validate projected repository authority: %w", err),
			workspace.Cleanup(),
		)
	}
	return projectedAuthority{authoritative: snapshot, projected: projected, workspace: workspace}, nil
}

func (authority projectedAuthority) verify(ctx context.Context) (repositoryfacts.Snapshot, error) {
	if authority.workspace == nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("%w: projected workspace is absent", ErrRepositoryAuthority)
	}
	if err := authority.workspace.VerifyExact(ctx); err != nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("%w: %v", ErrRepositoryAuthority, err)
	}
	after, err := repositoryfacts.BuildGitSnapshot(
		ctx, authority.authoritative.Root,
		repositoryfacts.SnapshotOptions{MaxFiles: maxRepositoryFiles},
	)
	if err != nil {
		return repositoryfacts.Snapshot{}, fmt.Errorf("reconcile repository objective snapshot: %w", err)
	}
	if after.ID != authority.authoritative.ID ||
		after.RepositoryID != authority.authoritative.RepositoryID ||
		after.HeadCommit != authority.authoritative.HeadCommit ||
		after.GitStateSHA256 != authority.authoritative.GitStateSHA256 ||
		after.Dirty != authority.authoritative.Dirty ||
		!reflect.DeepEqual(after.Files, authority.authoritative.Files) ||
		!reflect.DeepEqual(after.Exclusions, authority.authoritative.Exclusions) {
		return repositoryfacts.Snapshot{}, fmt.Errorf("%w: repository changed during read-only objective", ErrRepositoryAuthority)
	}
	return after, nil
}
