package changeapply

import (
	"context"
	"fmt"
)

// VerifyExactDelta proves that the live, unapplied changed-file post-image
// still equals the exact bounded delta sealed by Plan. It performs no write.
func (stage *StagedChange) VerifyExactDelta(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("verify exact repository change delta requires a context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("verify exact repository change delta: %w", err)
	}
	if stage == nil {
		return fmt.Errorf("verify exact repository change delta requires a staged change")
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
	if err := verifyStagedWorkspace(stage.deltaRoot, stage.deltaFiles); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("verify exact repository change delta: %w", err)
	}
	return nil
}
