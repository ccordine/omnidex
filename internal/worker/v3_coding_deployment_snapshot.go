package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

const directCodingDeploymentSnapshotDirectory = "deployment-snapshots-v1"

var errDirectCodingDeploymentSnapshotDrift = errors.New("deployment source snapshot drift")

type directCodingDeploymentSnapshotFile struct {
	Path   string
	SHA256 string
	Size   int64
}

type directCodingDeploymentWorkspaceSnapshot struct {
	Root       string
	SourceRoot string
	Identity   directCodingDeploymentWorkspaceIdentity
	files      []directCodingDeploymentSnapshotFile
}

func directCodingCreateVerifiedDeploymentWorkspaceSnapshot(
	root string,
	program directCodingProgram,
) (directCodingDeploymentWorkspaceSnapshot, error) {
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		return directCodingDeploymentWorkspaceSnapshot{}, err
	}
	identity, err := directCodingDeploymentWorkspaceIdentityFromAssembly(
		root, program.StackID, program.VersionProfileID, assembly,
	)
	if err != nil {
		return directCodingDeploymentWorkspaceSnapshot{}, err
	}
	return directCodingCreateDeploymentWorkspaceSnapshot(root, identity, assembly)
}

func directCodingOpenDeploymentWorkspaceSnapshot(
	root string,
	program directCodingProgram,
	expected directCodingDeploymentWorkspaceIdentity,
) (directCodingDeploymentWorkspaceSnapshot, error) {
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		return directCodingDeploymentWorkspaceSnapshot{}, err
	}
	return directCodingOpenDeploymentWorkspaceSnapshotFromAssembly(
		root, program.StackID, program.VersionProfileID, assembly, expected,
	)
}

func directCodingOpenDeploymentWorkspaceSnapshotFromAssembly(
	root, stackID, versionProfileID string,
	assembly directCodingAssembly,
	expected directCodingDeploymentWorkspaceIdentity,
) (directCodingDeploymentWorkspaceSnapshot, error) {
	identity, err := directCodingDeploymentWorkspaceIdentityForAssembly(
		stackID, versionProfileID, assembly,
	)
	if err != nil {
		return directCodingDeploymentWorkspaceSnapshot{}, err
	}
	if identity != expected {
		return directCodingDeploymentWorkspaceSnapshot{}, fmt.Errorf(
			"%w: compiled assembly differs from persisted deployment authority",
			errDirectCodingDeploymentSnapshotDrift,
		)
	}
	snapshot, err := directCodingDeploymentWorkspaceSnapshotAuthority(root, identity, assembly)
	if err != nil {
		return directCodingDeploymentWorkspaceSnapshot{}, err
	}
	if err := snapshot.VerifyExact(); err != nil {
		return directCodingDeploymentWorkspaceSnapshot{}, err
	}
	return snapshot, nil
}

func directCodingCreateDeploymentWorkspaceSnapshot(
	root string,
	identity directCodingDeploymentWorkspaceIdentity,
	assembly directCodingAssembly,
) (_ directCodingDeploymentWorkspaceSnapshot, returnErr error) {
	snapshot, err := directCodingDeploymentWorkspaceSnapshotAuthority(root, identity, assembly)
	if err != nil {
		return directCodingDeploymentWorkspaceSnapshot{}, err
	}
	if info, statErr := os.Lstat(snapshot.Root); statErr == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return directCodingDeploymentWorkspaceSnapshot{}, fmt.Errorf(
				"%w: deployment snapshot target is not one exact directory",
				errDirectCodingDeploymentSnapshotDrift,
			)
		}
		if err := snapshot.VerifyExact(); err != nil {
			return directCodingDeploymentWorkspaceSnapshot{}, err
		}
		return snapshot, nil
	} else if !os.IsNotExist(statErr) {
		return directCodingDeploymentWorkspaceSnapshot{}, fmt.Errorf("inspect deployment snapshot: %w", statErr)
	}
	parent, err := directCodingEnsureDeploymentSnapshotBoundary(root)
	if err != nil {
		return directCodingDeploymentWorkspaceSnapshot{}, err
	}
	staging, err := os.MkdirTemp(parent, ".staging-")
	if err != nil {
		return directCodingDeploymentWorkspaceSnapshot{}, fmt.Errorf("create deployment snapshot staging directory: %w", err)
	}
	defer func() {
		if staging != "" {
			returnErr = errors.Join(returnErr, directCodingRemoveSnapshotStaging(staging))
		}
	}()
	if err := directCodingWriteDeploymentSnapshot(staging, assembly); err != nil {
		return directCodingDeploymentWorkspaceSnapshot{}, err
	}
	staged := snapshot
	staged.Root = staging
	if err := directCodingVerifyDeploymentSnapshotTree(staged); err != nil {
		return directCodingDeploymentWorkspaceSnapshot{}, err
	}
	if err := os.Rename(staging, snapshot.Root); err != nil {
		if verifyErr := snapshot.VerifyExact(); verifyErr == nil {
			return snapshot, nil
		}
		return directCodingDeploymentWorkspaceSnapshot{}, fmt.Errorf("publish deployment snapshot: %w", err)
	}
	staging = ""
	if err := syncDirectCodingDeploymentDirectory(parent); err != nil {
		return directCodingDeploymentWorkspaceSnapshot{}, err
	}
	if err := snapshot.VerifyExact(); err != nil {
		return directCodingDeploymentWorkspaceSnapshot{}, err
	}
	return snapshot, nil
}

func directCodingDeploymentWorkspaceSnapshotAuthority(
	root string,
	identity directCodingDeploymentWorkspaceIdentity,
	assembly directCodingAssembly,
) (directCodingDeploymentWorkspaceSnapshot, error) {
	if err := validateV3CommandRoot(root); err != nil {
		return directCodingDeploymentWorkspaceSnapshot{}, fmt.Errorf("deployment snapshot source root: %w", err)
	}
	if !directCodingResolvedConfigHashPattern.MatchString(identity.WorkspaceSHA256) ||
		!directCodingResolvedConfigHashPattern.MatchString(identity.ComposeSHA256) ||
		identity.FileCount != len(assembly.Files) {
		return directCodingDeploymentWorkspaceSnapshot{}, fmt.Errorf("deployment snapshot identity is incomplete")
	}
	files := make([]directCodingDeploymentSnapshotFile, len(assembly.Files))
	for index, file := range assembly.Files {
		if err := file.validate(); err != nil {
			return directCodingDeploymentWorkspaceSnapshot{}, err
		}
		digest := sha256.Sum256([]byte(file.Content))
		files[index] = directCodingDeploymentSnapshotFile{
			Path: file.Path, SHA256: hex.EncodeToString(digest[:]), Size: int64(len(file.Content)),
		}
	}
	sort.Slice(files, func(left, right int) bool { return files[left].Path < files[right].Path })
	return directCodingDeploymentWorkspaceSnapshot{
		Root:       filepath.Join(root, ".omni", directCodingDeploymentSnapshotDirectory, identity.WorkspaceSHA256),
		SourceRoot: root, Identity: identity, files: files,
	}, nil
}

func (snapshot directCodingDeploymentWorkspaceSnapshot) VerifyExact() error {
	if snapshot.SourceRoot == "" || snapshot.Root != filepath.Join(
		snapshot.SourceRoot, ".omni", directCodingDeploymentSnapshotDirectory,
		snapshot.Identity.WorkspaceSHA256,
	) || len(snapshot.files) != snapshot.Identity.FileCount {
		return fmt.Errorf("%w: deployment snapshot authority is incomplete", errDirectCodingDeploymentSnapshotDrift)
	}
	if err := validateV3CommandRoot(snapshot.Root); err != nil {
		return fmt.Errorf("%w: deployment snapshot root is unsafe: %v", errDirectCodingDeploymentSnapshotDrift, err)
	}
	if err := directCodingVerifyDeploymentSnapshotTree(snapshot); err != nil {
		return fmt.Errorf("%w: %v", errDirectCodingDeploymentSnapshotDrift, err)
	}
	return nil
}

func directCodingVerifyDeploymentSnapshotTree(
	snapshot directCodingDeploymentWorkspaceSnapshot,
) error {
	expectedFiles := make(map[string]directCodingDeploymentSnapshotFile, len(snapshot.files))
	expectedDirectories := map[string]struct{}{".": {}}
	for _, file := range snapshot.files {
		expectedFiles[file.Path] = file
		for parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(file.Path))); parent != "."; parent = filepath.ToSlash(filepath.Dir(filepath.FromSlash(parent))) {
			expectedDirectories[parent] = struct{}{}
		}
	}
	seenFiles := make(map[string]struct{}, len(expectedFiles))
	err := filepath.WalkDir(snapshot.Root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(snapshot.Root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if _, exists := expectedDirectories[relative]; !exists || info.Mode().Perm() != 0o555 || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("deployment snapshot contains unexpected or mutable directory %q", relative)
			}
			return nil
		}
		expected, exists := expectedFiles[relative]
		if !exists || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o444 || info.Size() != expected.Size {
			return fmt.Errorf("deployment snapshot contains unexpected or mutable file %q", relative)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read deployment snapshot file %q: %w", relative, err)
		}
		digest := sha256.Sum256(content)
		if hex.EncodeToString(digest[:]) != expected.SHA256 {
			return fmt.Errorf("deployment snapshot file %q differs from sealed bytes", relative)
		}
		seenFiles[relative] = struct{}{}
		return nil
	})
	if err != nil {
		return err
	}
	if len(seenFiles) != len(expectedFiles) {
		return fmt.Errorf("deployment snapshot file inventory is incomplete")
	}
	return nil
}
