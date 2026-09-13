package worker

import (
	"fmt"
	"os"
	"path/filepath"
)

func writeDirectCodingStageFile(root string, file directCodingFileTask) error {
	if _, err := requireExactDirectCodingPath(file.Path); err != nil {
		return err
	}
	target := filepath.Join(root, filepath.FromSlash(file.Path))
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return fmt.Errorf("create staged directory for %s: %w", file.Path, err)
	}
	mode := os.FileMode(file.Mode)
	if mode == 0 || mode&^os.FileMode(0o777) != 0 {
		return fmt.Errorf("staged source %s has invalid mode", file.Path)
	}
	if err := os.WriteFile(target, file.Content, mode); err != nil {
		return fmt.Errorf("write staged source %s: %w", file.Path, err)
	}
	return nil
}
