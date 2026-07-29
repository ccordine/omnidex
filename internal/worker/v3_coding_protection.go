package worker

import (
	"bytes"
	"fmt"
	"os"
	"sort"
)

type directCodingProtectedPath struct {
	Path    string
	Exists  bool
	Content []byte
}

func snapshotDirectCodingProtectedPathList(
	root string,
	paths []string,
) (map[string]directCodingProtectedPath, error) {
	paths = cleanOrderedStrings(paths)
	protected := make(map[string]directCodingProtectedPath, len(paths))
	for _, path := range paths {
		target, err := resolveV3WorkspaceFile(root, path)
		if err != nil {
			return nil, err
		}
		content, err := os.ReadFile(target)
		if os.IsNotExist(err) {
			protected[path] = directCodingProtectedPath{Path: path}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("snapshot protected coding path %s: %w", path, err)
		}
		protected[path] = directCodingProtectedPath{
			Path: path, Exists: true, Content: append([]byte(nil), content...),
		}
	}
	return protected, nil
}

func validateDirectCodingAssemblyProtection(
	assembly directCodingAssembly,
	protected map[string]directCodingProtectedPath,
) error {
	for _, file := range assembly.Files {
		if err := rejectDirectCodingProtectedMutation(file.Path, protected); err != nil {
			return err
		}
	}
	for _, path := range assembly.DeletePaths {
		if err := rejectDirectCodingProtectedMutation(path, protected); err != nil {
			return err
		}
	}
	return nil
}

func rejectDirectCodingProtectedMutation(path string, protected map[string]directCodingProtectedPath) error {
	if _, exists := protected[path]; exists {
		return fmt.Errorf("coding path %s is server-protected by an explicit user requirement", path)
	}
	return nil
}

func validateDirectCodingProtectedPaths(root string, protected map[string]directCodingProtectedPath) error {
	paths := make([]string, 0, len(protected))
	for path := range protected {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		baseline := protected[path]
		target, err := resolveV3WorkspaceFile(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(target)
		if os.IsNotExist(err) {
			if baseline.Exists {
				return fmt.Errorf("server-protected coding path %s was deleted", path)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("verify protected coding path %s: %w", path, err)
		}
		if !baseline.Exists {
			return fmt.Errorf("server-protected coding path %s was created", path)
		}
		if !bytes.Equal(content, baseline.Content) {
			return fmt.Errorf("server-protected coding path %s was modified", path)
		}
	}
	return nil
}
