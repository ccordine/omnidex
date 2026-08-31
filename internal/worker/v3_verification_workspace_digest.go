package worker

import (
	"fmt"

	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

const (
	directCodingWorkspaceDigestMaxFiles = 4096
	directCodingWorkspaceDigestMaxBytes = 256 * 1024 * 1024
)

var directCodingWorkspaceDigestExcludedRoots = []string{
	".git", ".vite", "dist", "node_modules",
}

func directCodingAuthoritativeWorkspaceSHA256(
	fence *workspacefacts.MutationFence,
	root string,
) (string, error) {
	if fence == nil {
		return "", fmt.Errorf("authoritative workspace digest requires one exact mutation fence")
	}
	return fence.AuthoritativeWorkspaceSHA256(root, workspacefacts.WorkspaceDigestOptions{
		ExcludedRootNames: append([]string(nil), directCodingWorkspaceDigestExcludedRoots...),
		MaxPaths:          directCodingWorkspaceDigestMaxFiles,
		MaxBytes:          directCodingWorkspaceDigestMaxBytes,
	})
}
