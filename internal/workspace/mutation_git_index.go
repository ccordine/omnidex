package workspace

import (
	"context"
	"fmt"
	"os/exec"
)

// exactGitTrackedPath resolves only index provenance needed to predict the
// post-capture exclusion created when a tracked worktree file is deleted.
// The surrounding source snapshot remains the authority for all file bytes.
func exactGitTrackedPath(ctx context.Context, source Snapshot, relative string) (bool, error) {
	if source.Git == nil {
		return false, nil
	}
	command := exec.CommandContext(
		ctx, "git", "-C", source.Root, "--literal-pathspecs",
		"ls-files", "-z", "--cached", "--", relative,
	)
	output, err := command.Output()
	if err != nil {
		return false, fmt.Errorf("resolve Git index authority for %q: %w", relative, err)
	}
	if len(output) == 0 {
		return false, nil
	}
	if string(output) != relative+"\x00" {
		return false, fmt.Errorf("Git index returned ambiguous authority for %q", relative)
	}
	return true, nil
}
