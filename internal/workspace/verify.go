package workspace

import (
	"context"
	"fmt"
	"slices"
)

// VerifyExact re-captures only the paths owned by this operation.
func (snapshot Snapshot) VerifyExact(ctx context.Context) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("verify workspace source state: %w", err)
	}
	current, err := Capture(ctx, snapshot.Root, snapshot.Paths)
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
	if current.ID != snapshot.ID {
		return fmt.Errorf(
			"workspace source identity changed: expected %s, current %s",
			snapshot.ID,
			current.ID,
		)
	}
	return nil
}
