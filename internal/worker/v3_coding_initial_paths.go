package worker

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

type directCodingInitialPath struct {
	SHA256 [sha256.Size]byte
}

func snapshotDirectCodingInitialPaths(
	root string,
	paths []string,
) (map[string]directCodingInitialPath, error) {
	initial := make(map[string]directCodingInitialPath, len(paths))
	for index, value := range paths {
		normalized, err := normalizeDirectCodingPath(value)
		if err != nil {
			return nil, fmt.Errorf("initial workspace path %d: %w", index, err)
		}
		if normalized != value {
			return nil, fmt.Errorf(
				"initial workspace path %d must be exactly normalized: %q", index, value,
			)
		}
		if _, duplicate := initial[value]; duplicate {
			return nil, fmt.Errorf("initial workspace path %d duplicates %q", index, value)
		}
		digest, err := directCodingWorkspacePathSHA256(root, value)
		if err != nil {
			return nil, fmt.Errorf("snapshot initial workspace path %s: %w", value, err)
		}
		initial[value] = directCodingInitialPath{SHA256: digest}
	}
	return initial, nil
}

func directCodingWorkspacePathSHA256(root, path string) ([sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	target, err := resolveV3WorkspaceFile(root, path)
	if err != nil {
		return digest, err
	}
	file, err := os.Open(target)
	if err != nil {
		return digest, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return digest, err
	}
	if !info.Mode().IsRegular() {
		return digest, fmt.Errorf("workspace path %s is not a regular file", path)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return digest, err
	}
	copy(digest[:], hasher.Sum(nil))
	return digest, nil
}
