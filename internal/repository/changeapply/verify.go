package changeapply

import (
	"context"
	"fmt"
)

// VerifyExactWorkspace proves that the live, unapplied staging tree still
// equals the exact post-patch inventory sealed by Plan. It performs no write.
func (stage *StagedChange) VerifyExactWorkspace(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("verify exact repository change workspace requires a context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("verify exact repository change workspace: %w", err)
	}
	if stage == nil {
		return fmt.Errorf("verify exact repository change workspace requires a staged change")
	}
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if stage.closed {
		return fmt.Errorf("repository change stage %q is closed", stage.id)
	}
	if stage.applied {
		return fmt.Errorf("repository change stage %q was already applied", stage.id)
	}
	if digest([]byte(stage.patch)) != stage.patchSHA256 {
		return fmt.Errorf("repository change stage %q patch identity is invalid", stage.id)
	}
	if err := verifyStagedWorkspace(stage.workspace, stage.stagedFiles); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("verify exact repository change workspace: %w", err)
	}
	return nil
}
