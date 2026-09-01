package projectroot

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/model"
)

// ResolvePhysicalDirectory captures the exact physical identity of the
// directory in which a terminal client was invoked. It does not choose or
// create a project root.
func ResolvePhysicalDirectory(candidate string) (string, error) {
	if err := model.ValidateChannelWorkspaceRoot(candidate); err != nil {
		return "", fmt.Errorf("invoking directory: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve invoking directory %q: %w", candidate, err)
	}
	if err := model.ValidateChannelWorkspaceRoot(resolved); err != nil {
		return "", fmt.Errorf("resolved invoking directory: %w", err)
	}
	resolvedInfo, err := os.Lstat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect resolved invoking directory %q: %w", resolved, err)
	}
	if !resolvedInfo.IsDir() || resolvedInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("resolved invoking directory %q is not one exact directory", resolved)
	}
	candidateInfo, err := os.Stat(candidate)
	if err != nil {
		return "", fmt.Errorf("inspect invoking directory %q: %w", candidate, err)
	}
	if !candidateInfo.IsDir() || !os.SameFile(candidateInfo, resolvedInfo) {
		return "", fmt.Errorf("invoking directory %q changed during physical resolution", candidate)
	}
	directory, err := os.Open(resolved)
	if err != nil {
		return "", fmt.Errorf("open resolved invoking directory %q: %w", resolved, err)
	}
	openedInfo, statErr := directory.Stat()
	closeErr := directory.Close()
	if statErr != nil {
		return "", fmt.Errorf("inspect opened invoking directory %q: %w", resolved, statErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close opened invoking directory %q: %w", resolved, closeErr)
	}
	if !openedInfo.IsDir() || !os.SameFile(resolvedInfo, openedInfo) {
		return "", fmt.Errorf("opened invoking directory %q changed during physical resolution", resolved)
	}
	return resolved, nil
}
