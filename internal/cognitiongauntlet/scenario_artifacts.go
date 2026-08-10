package cognitiongauntlet

import (
	"encoding/json"
	"fmt"
	"os"
)

func ValidateManifestPair(public PublicManifest, oracle OracleManifest) error {
	if err := public.Validate(); err != nil {
		return err
	}
	if err := oracle.Validate(); err != nil {
		return err
	}
	if oracle.ScenarioID != public.Scenario.ID || oracle.PublicSHA256 != public.Scenario.SHA256 {
		return fmt.Errorf("public cognition scenario and private oracle authority do not match")
	}
	return nil
}

func SealPublicManifest(path string, manifest PublicManifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	return sealScenarioArtifact(path, manifest, "public cognition scenario")
}

func SealOracleManifest(path string, manifest OracleManifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	return sealScenarioArtifact(path, manifest, "private cognition oracle")
}

func LoadPublicManifest(path string) (PublicManifest, error) {
	var manifest PublicManifest
	if err := loadScenarioArtifact(path, &manifest, "public cognition scenario"); err != nil {
		return PublicManifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return PublicManifest{}, err
	}
	return manifest, nil
}

func LoadOracleManifest(path string) (OracleManifest, error) {
	var manifest OracleManifest
	if err := loadScenarioArtifact(path, &manifest, "private cognition oracle"); err != nil {
		return OracleManifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return OracleManifest{}, err
	}
	return manifest, nil
}

func sealScenarioArtifact(path string, value any, label string) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", label, err)
	}
	if len(raw) > 64*1024 {
		return fmt.Errorf("%s exceeds 65536 bytes", label)
	}
	if err := writeExclusiveAtomic(path, append(raw, '\n')); err != nil {
		return fmt.Errorf("seal %s: %w", label, err)
	}
	return nil
}

func loadScenarioArtifact(path string, target any, label string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", label, err)
	}
	if len(raw) > 64*1024+1 {
		return fmt.Errorf("%s exceeds 65536 bytes", label)
	}
	return decodeStrictJSON(raw, target, label)
}
