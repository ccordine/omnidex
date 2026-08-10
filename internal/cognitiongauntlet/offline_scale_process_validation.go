package cognitiongauntlet

import (
	"fmt"
	"os"
	"path/filepath"
)

func (config scaleEvaluatorProcessConfig) Validate() error {
	if config.Schema != scaleEvaluatorProcessSchemaV1 ||
		!validDigest(config.ExecutableSHA256) || !validDigest(config.SourceSHA256) ||
		!validCommitIdentity(config.OmnidexCommit) {
		return fmt.Errorf("offline Scale evaluator process authority is invalid")
	}
	if err := requireExact(config.PrivateOracleCredential, "private Scale credential", 512); err != nil {
		return err
	}
	inputs := []string{config.PrivateOraclePath, config.PublicBundlePath, config.EpisodePath}
	outputs := []string{config.EvaluationPath, config.ScaleEvidencePath}
	seen := make(map[string]struct{}, len(inputs)+len(outputs))
	for _, path := range append(inputs, outputs...) {
		if path == "" || filepath.Clean(path) != path {
			return fmt.Errorf("offline Scale evaluator path is inexact")
		}
		if _, duplicate := seen[path]; duplicate {
			return fmt.Errorf("offline Scale evaluator paths are duplicated")
		}
		seen[path] = struct{}{}
	}
	for _, path := range inputs {
		if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() ||
			info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("offline Scale evaluator input is unavailable")
		}
	}
	for _, path := range outputs {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return fmt.Errorf("offline Scale evaluator output already exists or is inaccessible")
		}
	}
	return nil
}
