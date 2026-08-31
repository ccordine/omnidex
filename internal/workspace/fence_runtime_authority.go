package workspace

import (
	"fmt"
	"os"
	"strconv"
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

// CommandWorkingDirectory returns a Linux fd-rooted cwd for a child process.
// The literal exactRoot remains the project identity and evidence value. The
// fence must outlive the child process so the descriptor remains authoritative.
func (fence *MutationFence) CommandWorkingDirectory(exactRoot string) (string, error) {
	if fence == nil {
		return "", fmt.Errorf("workspace command directory requires one mutation fence")
	}
	fence.mu.Lock()
	defer fence.mu.Unlock()
	if exactRoot == "" || exactRoot != fence.root {
		return "", fmt.Errorf("workspace command root differs from its mutation fence")
	}
	if _, err := fence.authoritativeRootLocked(); err != nil {
		return "", err
	}
	descriptorPath := "/proc/self/fd/" + strconv.Itoa(int(fence.directory.Fd()))
	info, err := os.Stat(descriptorPath)
	if err != nil {
		return "", fmt.Errorf("resolve fd-rooted workspace command directory: %w", err)
	}
	if !info.IsDir() || !os.SameFile(fence.rootInfo, info) {
		return "", fmt.Errorf("fd-rooted workspace command directory differs from fence authority")
	}
	return descriptorPath, nil
}
