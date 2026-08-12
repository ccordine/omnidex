package cognitiongauntlet

import (
	"fmt"
	"path/filepath"
)

func validateInferenceArtifactPaths(
	config inferenceProcessConfig,
	variant Variant,
) error {
	if err := validateOfflineOutputDirectories(
		config.PublicOutputDirectory, config.PrivateOutputDirectory,
	); err != nil {
		return err
	}
	wantEvidenceDirectory := config.PublicOutputDirectory
	if variant == VariantOracleEvidence {
		wantEvidenceDirectory = config.PrivateOutputDirectory
	}
	if config.PublicBundlePath != filepath.Join(
		config.PublicOutputDirectory, "inference-bootstrap.json",
	) || config.EpisodePath != filepath.Join(
		config.PublicOutputDirectory, "sealed-episode.json",
	) || config.EvidencePath != filepath.Join(
		wantEvidenceDirectory, "ablation-evidence.json",
	) {
		return fmt.Errorf("offline inference artifact paths differ from their typed output roots")
	}
	return nil
}
