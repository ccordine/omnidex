package cognitiongauntlet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func sealPreparedBrainEvidenceArtifact(
	publicDirectory string,
	artifact preparedBrainEvidenceArtifact,
	brain BrainFingerprint,
) (PreparedBrainEvidenceAuthority, error) {
	if _, err := artifact.verifyFor(brain); err != nil {
		return PreparedBrainEvidenceAuthority{}, err
	}
	raw, err := encodePreparedBrainEvidenceArtifact(artifact)
	if err != nil {
		return PreparedBrainEvidenceAuthority{}, err
	}
	digest := digestExactBytes(raw)
	path := filepath.Join(
		publicDirectory, "prepared-brain-evidence-"+digest+".json",
	)
	authority := PreparedBrainEvidenceAuthority{
		Schema: PreparedBrainEvidenceAuthoritySchemaV1,
		ID:     "prepared_brain_evidence_" + digest,
		SHA256: digest, Bytes: len(raw), Path: path,
		DiscoveryEvidence: artifact.Discovery.EvidenceManifest.Ref,
		BootstrapEvidence: artifact.Bootstrap.EvidenceManifest.Ref,
	}
	if err := authority.validateShape(); err != nil {
		return PreparedBrainEvidenceAuthority{}, err
	}
	if err := writeExclusiveAtomic(path, raw); err != nil {
		return PreparedBrainEvidenceAuthority{}, fmt.Errorf("seal prepared brain evidence: %w", err)
	}
	return authority, nil
}

func loadPreparedBrainEvidenceAuthority(
	authority PreparedBrainEvidenceAuthority,
	brain BrainFingerprint,
) (verifiedPreparedBrainEvidence, error) {
	if err := authority.validateShape(); err != nil {
		return verifiedPreparedBrainEvidence{}, err
	}
	pathInfo, err := os.Lstat(authority.Path)
	if err != nil {
		return verifiedPreparedBrainEvidence{}, fmt.Errorf("inspect prepared brain evidence: %w", err)
	}
	if !pathInfo.Mode().IsRegular() || pathInfo.Size() != int64(authority.Bytes) {
		return verifiedPreparedBrainEvidence{}, fmt.Errorf("prepared brain evidence is not one exact bounded regular file")
	}
	file, err := os.Open(authority.Path)
	if err != nil {
		return verifiedPreparedBrainEvidence{}, fmt.Errorf("open prepared brain evidence: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != int64(authority.Bytes) ||
		!os.SameFile(pathInfo, info) {
		return verifiedPreparedBrainEvidence{}, fmt.Errorf("prepared brain evidence is not one exact bounded regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxPreparedBrainEvidenceArtifactBytes+1))
	if err != nil {
		return verifiedPreparedBrainEvidence{}, fmt.Errorf("read bounded prepared brain evidence: %w", err)
	}
	if len(raw) != authority.Bytes || len(raw) > maxPreparedBrainEvidenceArtifactBytes {
		return verifiedPreparedBrainEvidence{}, fmt.Errorf("prepared brain evidence byte count changed")
	}
	var artifact preparedBrainEvidenceArtifact
	if err := decodeStrictJSON(raw, &artifact, "prepared brain evidence artifact"); err != nil {
		return verifiedPreparedBrainEvidence{}, err
	}
	canonical, err := encodePreparedBrainEvidenceArtifact(artifact)
	if err != nil || !bytes.Equal(raw, canonical) {
		return verifiedPreparedBrainEvidence{}, fmt.Errorf("prepared brain evidence artifact encoding changed")
	}
	verified, err := artifact.verifyFor(brain)
	if err != nil {
		return verifiedPreparedBrainEvidence{}, err
	}
	if digestExactBytes(raw) != authority.SHA256 ||
		artifact.Discovery.EvidenceManifest.Ref != authority.DiscoveryEvidence ||
		artifact.Bootstrap.EvidenceManifest.Ref != authority.BootstrapEvidence {
		return verifiedPreparedBrainEvidence{}, fmt.Errorf("prepared brain evidence differs from its configuration authority")
	}
	return verified, nil
}

func encodePreparedBrainEvidenceArtifact(
	artifact preparedBrainEvidenceArtifact,
) ([]byte, error) {
	raw, err := json.Marshal(artifact)
	if err != nil {
		return nil, fmt.Errorf("encode prepared brain evidence artifact: %w", err)
	}
	raw = append(raw, '\n')
	if len(raw) == 0 || len(raw) > maxPreparedBrainEvidenceArtifactBytes {
		return nil, fmt.Errorf("prepared brain evidence artifact exceeds its byte bound")
	}
	return raw, nil
}

func validateExactAbsolutePath(path, label string) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("%s path is not exact and absolute", label)
	}
	return nil
}
