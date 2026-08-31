package worker

import (
	"fmt"
	"os"
	"path/filepath"
)

func (s *directCodingSession) requiredArtifactFiles(
	paths []string,
	existing []directCodingFileTask,
) ([]directCodingFileTask, error) {
	known := make(map[string]struct{}, len(existing)+len(paths))
	for _, file := range existing {
		known[file.Path] = struct{}{}
	}
	files := append([]directCodingFileTask(nil), existing...)
	for _, relative := range paths {
		if _, exists := known[relative]; exists {
			continue
		}
		absolute := filepath.Join(s.root, filepath.FromSlash(relative))
		before, err := os.Lstat(absolute)
		if os.IsNotExist(err) {
			files = append(files, directCodingFileTask{
				Path: relative, Content: []byte{}, Mode: 0o644,
			})
			known[relative] = struct{}{}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect required artifact %q: %w", relative, err)
		}
		if !before.Mode().IsRegular() {
			return nil, fmt.Errorf("required artifact %q is not one regular file", relative)
		}
		known[relative] = struct{}{}
	}
	return files, nil
}
