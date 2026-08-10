package cognitiongauntlet

import (
	"fmt"
	"os"
	"path/filepath"
)

type MicrogauntletArtifactPaths struct {
	PublicManifest  string `json:"public_manifest"`
	PublicScenario  string `json:"public_scenario"`
	PrivateManifest string `json:"private_manifest"`
	PrivateOracle   string `json:"private_oracle"`
}

func (fixture MicrogauntletCase) SealArtifacts(
	surface Surface,
	paths MicrogauntletArtifactPaths,
) error {
	if err := validateArtifactPaths(paths); err != nil {
		return err
	}
	public, err := fixture.PublicManifest(surface)
	if err != nil {
		return err
	}
	oracle, err := fixture.oracleManifest()
	if err != nil {
		return err
	}
	if err := ValidateManifestPair(public, oracle); err != nil {
		return err
	}
	publicRaw, err := fixture.generated.MarshalPublicJSON()
	if err != nil {
		return fmt.Errorf("encode full public microgauntlet: %w", err)
	}
	oracleRaw, err := fixture.generated.MarshalOracleJSON()
	if err != nil {
		return fmt.Errorf("encode full private microgauntlet oracle: %w", err)
	}
	if err := SealPublicManifest(paths.PublicManifest, public); err != nil {
		return err
	}
	if err := writeExclusiveAtomic(paths.PublicScenario, append(publicRaw, '\n')); err != nil {
		return fmt.Errorf("seal full public microgauntlet: %w", err)
	}
	if err := SealOracleManifest(paths.PrivateManifest, oracle); err != nil {
		return err
	}
	if err := writeExclusiveAtomic(paths.PrivateOracle, append(oracleRaw, '\n')); err != nil {
		return fmt.Errorf("seal full private microgauntlet oracle: %w", err)
	}
	return nil
}

func validateArtifactPaths(paths MicrogauntletArtifactPaths) error {
	values := []string{
		paths.PublicManifest, paths.PublicScenario, paths.PrivateManifest, paths.PrivateOracle,
	}
	seen := make(map[string]struct{}, len(values))
	for _, path := range values {
		if path == "" || filepath.Clean(path) != path {
			return fmt.Errorf("microgauntlet artifact path must be exact")
		}
		if _, duplicate := seen[path]; duplicate {
			return fmt.Errorf("microgauntlet artifact paths must be distinct")
		}
		seen[path] = struct{}{}
		info, err := os.Stat(filepath.Dir(path))
		if err != nil || !info.IsDir() {
			return fmt.Errorf("microgauntlet artifact directory is unavailable")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			return fmt.Errorf("microgauntlet artifact target already exists or is inaccessible")
		}
	}
	publicDirectory := filepath.Dir(paths.PublicManifest)
	if filepath.Dir(paths.PublicScenario) != publicDirectory ||
		filepath.Dir(paths.PrivateManifest) != filepath.Dir(paths.PrivateOracle) {
		return fmt.Errorf("microgauntlet public and private artifacts require separate directories")
	}
	publicInfo, publicErr := os.Stat(publicDirectory)
	privateInfo, privateErr := os.Stat(filepath.Dir(paths.PrivateManifest))
	if publicErr != nil || privateErr != nil || os.SameFile(publicInfo, privateInfo) {
		return fmt.Errorf("microgauntlet public and private artifacts require separate directories")
	}
	return nil
}
