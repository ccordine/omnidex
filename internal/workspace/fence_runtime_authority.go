package workspace

import (
	"fmt"
)

// Reattest proves that exactRoot still names the directory and mount held by
// the fence. It does not resolve or replace the caller's exact path identity.
func (fence *MutationFence) Reattest(exactRoot string) error {
	if fence == nil {
		return fmt.Errorf("workspace mutation fence is unavailable")
	}
	fence.mu.Lock()
	defer fence.mu.Unlock()
	if exactRoot == "" || exactRoot != fence.root {
		return fmt.Errorf("workspace root differs from its mutation fence")
	}
	_, err := fence.authoritativeRootLocked()
	return err
}
