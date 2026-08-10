package cognitiongauntlet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInferenceBoundaryRequiresPrivateOutputsToRemainAbsent(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	paths := OfflinePromotionPaths{
		PrivateOracle: filepath.Join(directory, "private-oracle.json"),
		Evaluation:    filepath.Join(directory, "evaluation.json"),
		Receipt:       filepath.Join(directory, "receipt.json"),
	}
	if err := os.WriteFile(paths.PrivateOracle, []byte("sealed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := requireInferencePrivateOutputsAbsent(paths); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{paths.Evaluation, paths.Receipt} {
		if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := requireInferencePrivateOutputsAbsent(paths); err == nil {
			t.Fatalf("pre-populated private output %s was accepted", filepath.Base(path))
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
}
