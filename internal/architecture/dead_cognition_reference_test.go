package architecture

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParallelCognitionReferenceRuntimeIsAbsent(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	path := filepath.Join(root, "internal", "cognitionreference")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("parallel cognition reference runtime must be absent: %s", path)
	}
}
