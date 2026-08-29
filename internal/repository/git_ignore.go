package repository

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// RequireGitPathVisible proves that an exact normalized target path will be
// represented by the repository snapshot authority after mutation. An ignored
// path is terminal because writing it would create host state that the journal
// cannot subsequently observe.
func RequireGitPathVisible(ctx context.Context, root, relative string) error {
	if ctx == nil {
		return fmt.Errorf("Git visibility check requires a context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("Git visibility check: %w", err)
	}
	if err := validateRelativeRepositoryPath(relative); err != nil {
		return fmt.Errorf("Git visibility target: %w", err)
	}
	exactRoot, err := exactGitRoot(ctx, root, "")
	if err != nil {
		return err
	}
	command := exec.CommandContext(
		ctx, normalizedGitBin(""), "check-ignore", "--quiet", "--no-index", "--", relative,
	)
	command.Dir = exactRoot
	err = command.Run()
	if err == nil {
		return fmt.Errorf("repository target %q is ignored and cannot enter authoritative snapshots", relative)
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return nil
	}
	return fmt.Errorf("check repository target %q ignore authority: %w", relative, err)
}
