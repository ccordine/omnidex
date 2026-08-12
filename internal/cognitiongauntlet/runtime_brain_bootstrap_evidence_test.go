package cognitiongauntlet

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

func TestRuntimeBrainBootstrapEvidenceRoundTripsExactRawBodies(t *testing.T) {
	_, observed, brain := preparedBrainEvidenceFixture(t)
	attested, err := brain.attestedBrain()
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := cognitionpolicy.NewBrainBootstrap(attested, observed.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	episodePath := filepath.Join(t.TempDir(), "episode.json")
	artifact, authority, err := prepareRuntimeBrainBootstrapEvidence(
		episodePath, bootstrap, brain,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := sealRuntimeBrainBootstrapEvidence(
		episodePath, artifact, authority, brain,
	); err != nil {
		t.Fatal(err)
	}
	sealedRaw, err := os.ReadFile(runtimeBrainBootstrapEvidencePath(episodePath, authority))
	if err != nil {
		t.Fatal(err)
	}
	if digestExactBytes(sealedRaw) != authority.SHA256 {
		t.Fatal("runtime bootstrap evidence was not addressed by its exact artifact bytes")
	}
	loaded, err := loadRuntimeBrainBootstrapEvidence(episodePath, authority, brain)
	if err != nil {
		t.Fatal(err)
	}
	for index := range bootstrap.BootstrapEvidence.Operations {
		if !bytes.Equal(
			loaded.BootstrapEvidence.Operations[index].Request,
			bootstrap.BootstrapEvidence.Operations[index].Request,
		) || !bytes.Equal(
			loaded.BootstrapEvidence.Operations[index].ResponseCapture,
			bootstrap.BootstrapEvidence.Operations[index].ResponseCapture,
		) {
			t.Fatalf("runtime bootstrap operation %d raw bytes changed", index)
		}
	}

	path := runtimeBrainBootstrapEvidencePath(episodePath, authority)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)/2] ^= 1
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRuntimeBrainBootstrapEvidence(episodePath, authority, brain); err == nil {
		t.Fatal("altered runtime bootstrap evidence was accepted")
	}
}

func TestAblationEpisodeRejectsNormalizedBrainWithoutRuntimeBootstrapProof(t *testing.T) {
	manifest := validEpisodeManifest(mustRatGeneration(t), terminalTestPayload(t))
	manifest.Variant = VariantRawObservation
	if _, err := prepareEpisodeManifest(manifest); err == nil {
		t.Fatal("ablation episode accepted a normalized Brain without exact runtime bootstrap evidence")
	}
}
