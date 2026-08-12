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

func sealRuntimeProviderActivationEvidence(
	episodePath string,
	artifact runtimeProviderActivationEvidenceArtifact,
	authority RuntimeProviderActivationEvidenceAuthority,
	frozen BrainFingerprint,
) error {
	if _, err := artifact.verifyFor(frozen); err != nil {
		return err
	}
	raw, err := encodeRuntimeProviderActivationEvidenceArtifact(artifact)
	if err != nil {
		return err
	}
	if err := authority.Validate(); err != nil {
		return err
	}
	if digestExactBytes(raw) != authority.SHA256 || len(raw) != authority.Bytes ||
		artifact.Receipt.ID != authority.ObservationID ||
		artifact.Capture.EvidenceManifest.Ref != authority.Evidence {
		return fmt.Errorf("runtime provider activation artifact differs from its authority")
	}
	if err := writeExclusiveAtomic(runtimeProviderActivationEvidencePath(episodePath, authority), raw); err != nil {
		return fmt.Errorf("seal runtime provider activation evidence: %w", err)
	}
	return nil
}

func loadRuntimeProviderActivationEvidence(
	episodePath string,
	authority RuntimeProviderActivationEvidenceAuthority,
	frozen BrainFingerprint,
) (cognitionpolicy.ProviderProcessActivation, error) {
	if err := authority.Validate(); err != nil {
		return cognitionpolicy.ProviderProcessActivation{}, err
	}
	path := runtimeProviderActivationEvidencePath(episodePath, authority)
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return cognitionpolicy.ProviderProcessActivation{}, fmt.Errorf("inspect runtime provider activation evidence: %w", err)
	}
	if !pathInfo.Mode().IsRegular() || pathInfo.Size() != int64(authority.Bytes) {
		return cognitionpolicy.ProviderProcessActivation{}, fmt.Errorf("runtime provider activation evidence is not one exact regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return cognitionpolicy.ProviderProcessActivation{}, fmt.Errorf("open runtime provider activation evidence: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != int64(authority.Bytes) ||
		!os.SameFile(pathInfo, info) {
		return cognitionpolicy.ProviderProcessActivation{}, fmt.Errorf("runtime provider activation evidence file changed")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxRuntimeProviderActivationEvidenceBytes+1))
	if err != nil {
		return cognitionpolicy.ProviderProcessActivation{}, fmt.Errorf("read runtime provider activation evidence: %w", err)
	}
	if len(raw) != authority.Bytes || len(raw) > maxRuntimeProviderActivationEvidenceBytes {
		return cognitionpolicy.ProviderProcessActivation{}, fmt.Errorf("runtime provider activation evidence byte count changed")
	}
	var artifact runtimeProviderActivationEvidenceArtifact
	if err := decodeStrictJSON(raw, &artifact, "runtime provider activation evidence"); err != nil {
		return cognitionpolicy.ProviderProcessActivation{}, err
	}
	canonical, err := encodeRuntimeProviderActivationEvidenceArtifact(artifact)
	if err != nil || !bytes.Equal(raw, canonical) {
		return cognitionpolicy.ProviderProcessActivation{}, fmt.Errorf("runtime provider activation evidence encoding changed")
	}
	activation, err := artifact.verifyFor(frozen)
	if err != nil {
		return cognitionpolicy.ProviderProcessActivation{}, err
	}
	if digestExactBytes(raw) != authority.SHA256 || artifact.Receipt.ID != authority.ObservationID ||
		artifact.Capture.EvidenceManifest.Ref != authority.Evidence {
		return cognitionpolicy.ProviderProcessActivation{}, fmt.Errorf("runtime provider activation evidence differs from its sealed authority")
	}
	return activation, nil
}

func encodeRuntimeProviderActivationEvidenceArtifact(
	artifact runtimeProviderActivationEvidenceArtifact,
) ([]byte, error) {
	raw, err := json.Marshal(artifact)
	if err != nil {
		return nil, fmt.Errorf("encode runtime provider activation evidence: %w", err)
	}
	raw = append(raw, '\n')
	if len(raw) == 0 || len(raw) > maxRuntimeProviderActivationEvidenceBytes {
		return nil, fmt.Errorf("runtime provider activation evidence exceeds its byte bound")
	}
	return raw, nil
}

func runtimeProviderActivationEvidencePath(
	episodePath string,
	authority RuntimeProviderActivationEvidenceAuthority,
) string {
	return filepath.Join(filepath.Dir(episodePath), authority.File)
}
