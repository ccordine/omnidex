package workspace

import (
	"context"
	"fmt"
	"slices"
)

// VerifyExact re-captures the exact workspace root and rejects any change to
// included authority. Excluded content bytes are intentionally outside the
// state identity, while additions and removals from the exclusion inventory
// still change that identity.
func (snapshot Snapshot) VerifyExact(ctx context.Context) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("verify workspace source state: %w", err)
	}
	current, err := Capture(ctx, snapshot.Root)
	if err != nil {
		return fmt.Errorf("verify workspace source state changed: %w", err)
	}
	if current.WorkspaceID != snapshot.WorkspaceID || current.Root != snapshot.Root {
		return fmt.Errorf("workspace source identity changed while verifying exact state")
	}
	if !slices.Equal(current.Entries, snapshot.Entries) {
		return fmt.Errorf(
			"workspace source state changed: expected %s, current %s",
			snapshot.ID,
			current.ID,
		)
	}
	if !slices.Equal(current.Exclusions, snapshot.Exclusions) {
		return fmt.Errorf("workspace exclusion authority changed while verifying exact state")
	}
	if current.ID != snapshot.ID {
		return fmt.Errorf(
			"workspace source identity changed: expected %s, current %s",
			snapshot.ID,
			current.ID,
		)
	}
	if !equalGitBinding(current.Git, snapshot.Git) {
		return fmt.Errorf("workspace Git binding changed while verifying exact state")
	}
	return nil
}

func equalGitBinding(left, right *GitBinding) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
