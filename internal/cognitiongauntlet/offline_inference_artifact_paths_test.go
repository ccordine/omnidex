package cognitiongauntlet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInferenceArtifactPathClassIsServerAuthoritative(t *testing.T) {
	publicRoot := t.TempDir()
	privateRoot := t.TempDir()
	if err := os.Chmod(privateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	config := inferenceProcessConfig{
		PublicOutputDirectory: publicRoot, PrivateOutputDirectory: privateRoot,
		PublicBundlePath: filepath.Join(publicRoot, "inference-bootstrap.json"),
		EpisodePath:      filepath.Join(publicRoot, "sealed-episode.json"),
		EvidencePath:     filepath.Join(publicRoot, "ablation-evidence.json"),
	}
	if err := validateInferenceArtifactPaths(config, VariantRawObservation); err != nil {
		t.Fatal(err)
	}
	config.EvidencePath = filepath.Join(privateRoot, "ablation-evidence.json")
	if err := validateInferenceArtifactPaths(config, VariantRawObservation); err == nil {
		t.Fatal("ordinary ablation evidence was accepted under the private output root")
	}
	if err := validateInferenceArtifactPaths(config, VariantOracleEvidence); err != nil {
		t.Fatal(err)
	}
	config.EvidencePath = filepath.Join(publicRoot, "ablation-evidence.json")
	if err := validateInferenceArtifactPaths(config, VariantOracleEvidence); err == nil {
		t.Fatal("oracle-contaminated evidence was accepted under the public output root")
	}
}
