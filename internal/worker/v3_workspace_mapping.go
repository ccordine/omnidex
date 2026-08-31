package worker

import (
	"fmt"
	"path/filepath"
	"strings"
)

// resolveV3WorkspaceRoot preserves the exact typed root carried by the job.
// Filesystem existence and kind are checked only when the reconciler consumes
// the root.
func resolveV3WorkspaceRoot(requested string) (string, error) {
	if requested == "" || requested != strings.TrimSpace(requested) {
		return "", fmt.Errorf("requested workspace root must be canonical text")
	}
	if !filepath.IsAbs(requested) || filepath.Clean(requested) != requested {
		return "", fmt.Errorf("requested workspace root must be one canonical absolute path")
	}
	return requested, nil
}
