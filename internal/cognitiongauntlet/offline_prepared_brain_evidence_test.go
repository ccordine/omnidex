package cognitiongauntlet

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestPreparedBrainEvidenceArtifactRoundTripsExactDiscoveryAndBootstrapBodies(t *testing.T) {
	discovery, bootstrap, brain := preparedBrainEvidenceFixture(t)
	artifact, err := newPreparedBrainEvidenceArtifact(discovery, bootstrap, brain)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := sealPreparedBrainEvidenceArtifact(t.TempDir(), artifact, brain)
	if err != nil {
		t.Fatal(err)
	}
	rawArtifact, err := os.ReadFile(authority.Path)
	if err != nil {
		t.Fatal(err)
	}
	if digestExactBytes(rawArtifact) != authority.SHA256 {
		t.Fatal("prepared Brain evidence filename was not addressed by the exact artifact bytes")
	}
	loaded, err := loadPreparedBrainEvidenceAuthority(authority, brain)
	if err != nil {
		t.Fatal(err)
	}
	for index := range discovery.Evidence.Operations {
		if !bytes.Equal(
			loaded.discovery.Evidence.Operations[index].Request,
			discovery.Evidence.Operations[index].Request,
		) || !bytes.Equal(
			loaded.discovery.Evidence.Operations[index].ResponseCapture,
			discovery.Evidence.Operations[index].ResponseCapture,
		) || !bytes.Equal(
			loaded.bootstrap.Evidence.Operations[index].Request,
			bootstrap.Evidence.Operations[index].Request,
		) || !bytes.Equal(
			loaded.bootstrap.Evidence.Operations[index].ResponseCapture,
			bootstrap.Evidence.Operations[index].ResponseCapture,
		) {
			t.Fatalf("provider identity operation %d raw bytes changed", index)
		}
	}
}

func TestPreparedBrainEvidenceAuthorityRejectsMissingAlteredAndSwappedEvidence(t *testing.T) {
	discovery, bootstrap, brain := preparedBrainEvidenceFixture(t)
	artifact, err := newPreparedBrainEvidenceArtifact(discovery, bootstrap, brain)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	authority, err := sealPreparedBrainEvidenceArtifact(directory, artifact, brain)
	if err != nil {
		t.Fatal(err)
	}

	missing := authority
	missing.Path = filepath.Join(directory, "missing.json")
	if _, err := loadPreparedBrainEvidenceAuthority(missing, brain); err == nil {
		t.Fatal("missing prepared brain evidence was accepted")
	}

	raw, err := os.ReadFile(authority.Path)
	if err != nil {
		t.Fatal(err)
	}
	alteredPath := filepath.Join(directory, "altered.json")
	altered := append([]byte(nil), raw...)
	altered[len(altered)/2] ^= 1
	if err := os.WriteFile(alteredPath, altered, 0o600); err != nil {
		t.Fatal(err)
	}
	changed := authority
	changed.Path = alteredPath
	if _, err := loadPreparedBrainEvidenceAuthority(changed, brain); err == nil {
		t.Fatal("altered prepared brain evidence was accepted")
	}

	symlinkPath := filepath.Join(directory, "evidence-link.json")
	if err := os.Symlink(authority.Path, symlinkPath); err != nil {
		t.Fatal(err)
	}
	linked := authority
	linked.Path = symlinkPath
	if _, err := loadPreparedBrainEvidenceAuthority(linked, brain); err == nil {
		t.Fatal("symlinked prepared brain evidence was accepted")
	}

	swapped := artifact
	swapped.Discovery, swapped.Bootstrap = swapped.Bootstrap, swapped.Discovery
	if _, err := finalizePreparedBrainEvidenceArtifact(swapped, brain); err == nil {
		t.Fatal("bootstrap and discovery evidence roles were interchangeable")
	}
}

func TestOfflinePromotionRequiresFreshlyVerifiablePreparedBrainEvidence(t *testing.T) {
	request := offlinePrepareTestRequest(t, OfflineExperimentRun)
	executable := filepath.Join(t.TempDir(), "cognition-gauntlet")
	if err := os.WriteFile(executable, []byte("exact-release-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	discovery, bootstrap, host := offlinePrepareTestEvidence(t)
	prepared, err := prepareOfflineExperiment(
		request, discovery, bootstrap, host, executable,
		strings.Repeat("a", 40), strings.Repeat("b", 64), strings.Repeat("c", 64), "v0.5.0",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadPreparedBrainEvidenceAuthority(
		prepared.promotion.PreparedBrainEvidence,
		prepared.promotion.RatGeneration.Fixed.Brain,
	); err != nil {
		t.Fatalf("fresh verifier could not reconstruct prepared brain: %v", err)
	}

	withoutEvidence := prepared.promotion
	withoutEvidence.PreparedBrainEvidence = PreparedBrainEvidenceAuthority{}
	if err := withoutEvidence.Validate(); err == nil {
		t.Fatal("normalized-only offline promotion configuration was accepted")
	}
}

func preparedBrainEvidenceFixture(
	t *testing.T,
) (llm.ObservedProviderIdentity, llm.ObservedProviderIdentity, BrainFingerprint) {
	t.Helper()
	bootstrap, _ := offlinePrepareTestAttestations(t)
	selection := llm.ProviderIdentitySelection{
		Model:              bootstrap.Attestation.Model,
		NativeContextLimit: bootstrap.Attestation.NativeContextLimit,
	}
	challenge, err := llm.DeriveProviderIdentityDiscoveryChallenge(
		offlineProviderDiscoveryScopeV1, selection,
	)
	if err != nil {
		t.Fatal(err)
	}
	discovery, err := llm.NewObservedProviderIdentity(
		bootstrap.Observation.ObservedAt.Add(-time.Second), bootstrap.Attestation,
		bootstrap.Evidence, challenge,
	)
	if err != nil {
		t.Fatal(err)
	}
	return discovery, bootstrap, mustRatGeneration(t).Fixed.Brain
}

func testPreparedBrainEvidenceAuthority(
	t *testing.T,
	brain BrainFingerprint,
	publicDirectory string,
) PreparedBrainEvidenceAuthority {
	t.Helper()
	discovery, bootstrap, _ := preparedBrainEvidenceFixture(t)
	artifact, err := newPreparedBrainEvidenceArtifact(discovery, bootstrap, brain)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := sealPreparedBrainEvidenceArtifact(publicDirectory, artifact, brain)
	if err != nil {
		t.Fatal(err)
	}
	return authority
}
