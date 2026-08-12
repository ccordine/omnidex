package cognitiongauntlet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

func sealRuntimeBrainBootstrapEvidence(
	episodePath string,
	artifact runtimeBrainBootstrapEvidenceArtifact,
	authority RuntimeBrainBootstrapEvidenceAuthority,
	frozen BrainFingerprint,
) error {
	if _, err := artifact.verifyFor(frozen); err != nil {
		return err
	}
	raw, err := encodeRuntimeBrainBootstrapEvidenceArtifact(artifact)
	if err != nil {
		return err
	}
	if err := authority.Validate(); err != nil {
		return err
	}
	if digestExactBytes(raw) != authority.SHA256 || len(raw) != authority.Bytes ||
		artifact.Capture.EvidenceManifest.Ref != authority.Evidence {
		return fmt.Errorf("runtime Brain bootstrap artifact differs from its authority")
	}
	if err := writeExclusiveAtomic(runtimeBrainBootstrapEvidencePath(episodePath, authority), raw); err != nil {
		return fmt.Errorf("seal runtime Brain bootstrap evidence: %w", err)
	}
	return nil
}

func loadRuntimeBrainBootstrapEvidence(
	episodePath string,
	authority RuntimeBrainBootstrapEvidenceAuthority,
	frozen BrainFingerprint,
) (cognitionpolicy.BrainBootstrap, error) {
	if err := authority.Validate(); err != nil {
		return cognitionpolicy.BrainBootstrap{}, err
	}
	path := runtimeBrainBootstrapEvidencePath(episodePath, authority)
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return cognitionpolicy.BrainBootstrap{}, fmt.Errorf("inspect runtime Brain bootstrap evidence: %w", err)
	}
	if !pathInfo.Mode().IsRegular() || pathInfo.Size() != int64(authority.Bytes) {
		return cognitionpolicy.BrainBootstrap{}, fmt.Errorf("runtime Brain bootstrap evidence is not one exact regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return cognitionpolicy.BrainBootstrap{}, fmt.Errorf("open runtime Brain bootstrap evidence: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != int64(authority.Bytes) ||
		!os.SameFile(pathInfo, info) {
		return cognitionpolicy.BrainBootstrap{}, fmt.Errorf("runtime Brain bootstrap evidence file changed")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxRuntimeBrainBootstrapEvidenceBytes+1))
	if err != nil {
		return cognitionpolicy.BrainBootstrap{}, fmt.Errorf("read runtime Brain bootstrap evidence: %w", err)
	}
	if len(raw) != authority.Bytes || len(raw) > maxRuntimeBrainBootstrapEvidenceBytes {
		return cognitionpolicy.BrainBootstrap{}, fmt.Errorf("runtime Brain bootstrap evidence byte count changed")
	}
	var artifact runtimeBrainBootstrapEvidenceArtifact
	if err := decodeStrictJSON(raw, &artifact, "runtime Brain bootstrap evidence"); err != nil {
		return cognitionpolicy.BrainBootstrap{}, err
	}
	canonical, err := encodeRuntimeBrainBootstrapEvidenceArtifact(artifact)
	if err != nil || !bytes.Equal(raw, canonical) {
		return cognitionpolicy.BrainBootstrap{}, fmt.Errorf("runtime Brain bootstrap evidence encoding changed")
	}
	bootstrap, err := artifact.verifyFor(frozen)
	if err != nil {
		return cognitionpolicy.BrainBootstrap{}, err
	}
	if digestExactBytes(raw) != authority.SHA256 ||
		artifact.Capture.EvidenceManifest.Ref != authority.Evidence {
		return cognitionpolicy.BrainBootstrap{}, fmt.Errorf("runtime Brain bootstrap evidence differs from its sealed authority")
	}
	return bootstrap, nil
}

func encodeRuntimeBrainBootstrapEvidenceArtifact(
	artifact runtimeBrainBootstrapEvidenceArtifact,
) ([]byte, error) {
	raw, err := json.Marshal(artifact)
	if err != nil {
		return nil, fmt.Errorf("encode runtime Brain bootstrap evidence: %w", err)
	}
	raw = append(raw, '\n')
	if len(raw) == 0 || len(raw) > maxRuntimeBrainBootstrapEvidenceBytes {
		return nil, fmt.Errorf("runtime Brain bootstrap evidence exceeds its byte bound")
	}
	return raw, nil
}

func runtimeBrainBootstrapEvidencePath(
	episodePath string,
	authority RuntimeBrainBootstrapEvidenceAuthority,
) string {
	return filepath.Join(filepath.Dir(episodePath), authority.File)
}
