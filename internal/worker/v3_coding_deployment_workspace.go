package worker

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const directCodingDeploymentComposePath = "docker-compose.yml"

type directCodingDeploymentWorkspaceIdentity struct {
	WorkspaceSHA256 string
	ComposeSHA256   string
	FileCount       int
}

func directCodingVerifiedDeploymentWorkspaceIdentity(
	root string,
	program directCodingProgram,
) (directCodingDeploymentWorkspaceIdentity, error) {
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		return directCodingDeploymentWorkspaceIdentity{}, err
	}
	return directCodingDeploymentWorkspaceIdentityFromAssembly(
		root, program.StackID, program.VersionProfileID, assembly,
	)
}

func directCodingDeploymentWorkspaceIdentityFromAssembly(
	root, stackID, versionProfileID string,
	assembly directCodingAssembly,
) (directCodingDeploymentWorkspaceIdentity, error) {
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return directCodingDeploymentWorkspaceIdentity{}, fmt.Errorf("deployment workspace requires one normalized absolute root")
	}
	if stackID == "" || versionProfileID == "" {
		return directCodingDeploymentWorkspaceIdentity{}, fmt.Errorf("deployment workspace requires exact stack and version authority")
	}
	if err := assembly.validate(); err != nil {
		return directCodingDeploymentWorkspaceIdentity{}, err
	}
	identity, err := directCodingDeploymentWorkspaceIdentityForAssembly(
		stackID, versionProfileID, assembly,
	)
	if err != nil {
		return directCodingDeploymentWorkspaceIdentity{}, err
	}
	files := append([]directCodingFileTask(nil), assembly.Files...)
	sort.Slice(files, func(left, right int) bool { return files[left].Path < files[right].Path })
	for _, file := range files {
		target, err := resolveV3WorkspaceFile(root, file.Path)
		if err != nil {
			return directCodingDeploymentWorkspaceIdentity{}, err
		}
		info, err := os.Lstat(target)
		if err != nil {
			return directCodingDeploymentWorkspaceIdentity{}, fmt.Errorf("inspect deployed source %s: %w", file.Path, err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return directCodingDeploymentWorkspaceIdentity{}, fmt.Errorf("deployed source %s is not one regular non-symlink file", file.Path)
		}
		content, err := os.ReadFile(target)
		if err != nil {
			return directCodingDeploymentWorkspaceIdentity{}, fmt.Errorf("read deployed source %s: %w", file.Path, err)
		}
		if !bytes.Equal(content, []byte(file.Content)) {
			return directCodingDeploymentWorkspaceIdentity{}, fmt.Errorf("deployed source %s differs from the verified in-memory assembly", file.Path)
		}
	}
	return identity, nil
}

func directCodingDeploymentWorkspaceIdentityForAssembly(
	stackID, versionProfileID string,
	assembly directCodingAssembly,
) (directCodingDeploymentWorkspaceIdentity, error) {
	if stackID == "" || versionProfileID == "" {
		return directCodingDeploymentWorkspaceIdentity{}, fmt.Errorf("deployment workspace requires exact stack and version authority")
	}
	if err := assembly.validate(); err != nil {
		return directCodingDeploymentWorkspaceIdentity{}, err
	}
	files := append([]directCodingFileTask(nil), assembly.Files...)
	sort.Slice(files, func(left, right int) bool { return files[left].Path < files[right].Path })
	hasher := sha256.New()
	writeDeploymentIdentityField(hasher, []byte("omnidex.deployment-workspace.v1"))
	writeDeploymentIdentityField(hasher, []byte(stackID))
	writeDeploymentIdentityField(hasher, []byte(versionProfileID))
	composeSHA256 := ""
	for _, file := range files {
		content := []byte(file.Content)
		writeDeploymentIdentityField(hasher, []byte(file.Path))
		writeDeploymentIdentityField(hasher, content)
		if file.Path == directCodingDeploymentComposePath {
			digest := sha256.Sum256(content)
			composeSHA256 = hex.EncodeToString(digest[:])
		}
	}
	if composeSHA256 == "" {
		return directCodingDeploymentWorkspaceIdentity{}, fmt.Errorf("deployment workspace omits %s", directCodingDeploymentComposePath)
	}
	return directCodingDeploymentWorkspaceIdentity{
		WorkspaceSHA256: hex.EncodeToString(hasher.Sum(nil)),
		ComposeSHA256:   composeSHA256,
		FileCount:       len(files),
	}, nil
}

type directCodingDeploymentIdentityWriter interface {
	Write([]byte) (int, error)
}

func writeDeploymentIdentityField(writer directCodingDeploymentIdentityWriter, value []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = writer.Write(size[:])
	_, _ = writer.Write(value)
}
