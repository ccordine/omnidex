package changeapply

import (
	"context"
	"fmt"
	"os"
	"sync"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

// SnapshotWorkspace is a disposable exact projection of Snapshot.Files. It never
// copies Git metadata, ignored files, .omni state, or snapshot exclusions.
type SnapshotWorkspace struct {
	mu     sync.Mutex
	root   string
	files  []stagedFileAuthority
	closed bool
}

func NewSnapshotWorkspace(
	ctx context.Context,
	snapshot repositoryfacts.Snapshot,
) (*SnapshotWorkspace, error) {
	if ctx == nil {
		return nil, fmt.Errorf("repository snapshot workspace requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("repository snapshot workspace: %w", err)
	}
	if err := snapshot.Validate(); err != nil {
		return nil, fmt.Errorf("repository snapshot workspace authority: %w", err)
	}
	if err := verifyAuthoritativeSnapshot(ctx, snapshot.Root, snapshot.ID); err != nil {
		return nil, fmt.Errorf("repository snapshot workspace source: %w", err)
	}
	root, err := stageSnapshot(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	workspace := &SnapshotWorkspace{
		root: root, files: stagedFileAuthorities(snapshot, nil),
	}
	if err := workspace.VerifyExact(ctx); err != nil {
		return nil, joinCleanupError(err, workspace.Cleanup())
	}
	return workspace, nil
}

func (workspace *SnapshotWorkspace) Root() string {
	if workspace == nil {
		return ""
	}
	workspace.mu.Lock()
	defer workspace.mu.Unlock()
	if workspace.closed {
		return ""
	}
	return workspace.root
}

func (workspace *SnapshotWorkspace) VerifyExact(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("verify repository snapshot workspace requires a context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("verify repository snapshot workspace: %w", err)
	}
	if workspace == nil {
		return fmt.Errorf("verify repository snapshot workspace requires a workspace")
	}
	workspace.mu.Lock()
	defer workspace.mu.Unlock()
	if workspace.closed || workspace.root == "" {
		return fmt.Errorf("repository snapshot workspace is closed")
	}
	if err := verifyStagedWorkspace(workspace.root, workspace.files); err != nil {
		return fmt.Errorf("verify repository snapshot workspace: %w", err)
	}
	return nil
}

func (workspace *SnapshotWorkspace) Cleanup() error {
	if workspace == nil {
		return nil
	}
	workspace.mu.Lock()
	defer workspace.mu.Unlock()
	if workspace.closed {
		return nil
	}
	if workspace.root == "" {
		return fmt.Errorf("repository snapshot workspace has no exact cleanup root")
	}
	if err := os.RemoveAll(workspace.root); err != nil {
		return fmt.Errorf("clean repository snapshot workspace: %w", err)
	}
	workspace.closed = true
	return nil
}
