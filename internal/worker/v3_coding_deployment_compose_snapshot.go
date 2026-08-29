package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/operation"
)

type directCodingDeploymentComposeSnapshotAuthority struct {
	Root          string
	ComposeSHA256 string
}

func directCodingOpenPersistedDeploymentComposeSnapshot(
	sourceRoot, workspaceSHA256, composeSHA256 string,
) (directCodingDeploymentComposeSnapshotAuthority, error) {
	if err := validateV3CommandRoot(sourceRoot); err != nil {
		return directCodingDeploymentComposeSnapshotAuthority{}, fmt.Errorf(
			"persisted deployment snapshot source root: %w", err,
		)
	}
	if !directCodingResolvedConfigHashPattern.MatchString(workspaceSHA256) {
		return directCodingDeploymentComposeSnapshotAuthority{}, fmt.Errorf(
			"persisted deployment snapshot workspace identity is malformed",
		)
	}
	return directCodingDeploymentComposeSnapshotAuthorityAtRoot(
		filepath.Join(
			sourceRoot, ".omni", directCodingDeploymentSnapshotDirectory, workspaceSHA256,
		),
		composeSHA256,
	)
}

func directCodingDeploymentComposeSnapshotAuthorityAtRoot(
	root, composeSHA256 string,
) (directCodingDeploymentComposeSnapshotAuthority, error) {
	authority := directCodingDeploymentComposeSnapshotAuthority{
		Root: root, ComposeSHA256: composeSHA256,
	}
	if err := authority.VerifyExact(); err != nil {
		return directCodingDeploymentComposeSnapshotAuthority{}, err
	}
	return authority, nil
}

func (authority directCodingDeploymentComposeSnapshotAuthority) VerifyExact() error {
	if authority.Root == "" ||
		!directCodingResolvedConfigHashPattern.MatchString(authority.ComposeSHA256) {
		return fmt.Errorf(
			"%w: persisted deployment Compose snapshot authority is incomplete",
			errDirectCodingDeploymentSnapshotDrift,
		)
	}
	if err := validateV3CommandRoot(authority.Root); err != nil {
		return fmt.Errorf(
			"%w: persisted deployment Compose snapshot root is unsafe: %v",
			errDirectCodingDeploymentSnapshotDrift, err,
		)
	}
	rootInfo, err := os.Lstat(authority.Root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 ||
		rootInfo.Mode().Perm() != 0o555 {
		return fmt.Errorf(
			"%w: persisted deployment Compose snapshot root is unavailable or mutable",
			errDirectCodingDeploymentSnapshotDrift,
		)
	}
	composePath := filepath.Join(authority.Root, directCodingDeploymentComposePath)
	info, err := os.Lstat(composePath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Mode().Perm() != 0o444 {
		return fmt.Errorf(
			"%w: persisted deployment Compose source is unavailable or mutable",
			errDirectCodingDeploymentSnapshotDrift,
		)
	}
	content, err := os.ReadFile(composePath)
	if err != nil {
		return fmt.Errorf(
			"%w: read persisted deployment Compose source: %v",
			errDirectCodingDeploymentSnapshotDrift, err,
		)
	}
	digest := sha256.Sum256(content)
	if hex.EncodeToString(digest[:]) != authority.ComposeSHA256 {
		return fmt.Errorf(
			"%w: persisted deployment Compose bytes differ",
			errDirectCodingDeploymentSnapshotDrift,
		)
	}
	return nil
}

func (authority directCodingDeploymentComposeSnapshotAuthority) ExecutionRoot() (string, error) {
	if err := authority.VerifyExact(); err != nil {
		return "", err
	}
	return authority.Root, nil
}

func executeDirectCodingComposeSnapshotBoundCommand(
	authority directCodingDeploymentComposeSnapshotAuthority,
	expectedRoot string,
	execute directCodingSnapshotCommandExecutor,
) (operation.Result, error) {
	if execute == nil {
		return operation.Result{}, fmt.Errorf("deployment rollback command executor is required")
	}
	root, err := authority.ExecutionRoot()
	if err != nil {
		return operation.Result{}, err
	}
	if root != expectedRoot {
		return operation.Result{}, fmt.Errorf(
			"%w: deployment rollback root differs from persisted Compose snapshot",
			errDirectCodingDeploymentSnapshotDrift,
		)
	}
	return execute(root)
}
