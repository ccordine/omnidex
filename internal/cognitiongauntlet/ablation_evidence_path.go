package cognitiongauntlet

import (
	"fmt"
	"os"
	"path/filepath"
)

func validateAblationEvidenceOutputPath(evidencePath, episodePath string) error {
	if evidencePath == "" || filepath.Clean(evidencePath) != evidencePath ||
		evidencePath == episodePath {
		return fmt.Errorf("ablation evidence output path is inexact")
	}
	if _, err := os.Lstat(evidencePath); !os.IsNotExist(err) {
		return fmt.Errorf("ablation evidence output already exists or is inaccessible")
	}
	info, err := os.Stat(filepath.Dir(evidencePath))
	if err != nil || !info.IsDir() {
		return fmt.Errorf("ablation evidence output directory is unavailable")
	}
	return nil
}
