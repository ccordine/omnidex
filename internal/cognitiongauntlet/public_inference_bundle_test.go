package cognitiongauntlet

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicInferenceBundleContainsNoPrivateEvaluationAuthority(t *testing.T) {
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[4])
	if err != nil {
		t.Fatal(err)
	}
	paired, err := fixture.PairedAuthority(
		SurfaceSymbolic, mustRatGeneration(t), 1, transferTestFingerprint(),
	)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := NewPublicInferenceBundle(fixture, paired)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	assertNoPrivateEvaluationAuthority(t, raw, paired)
	for _, forbidden := range []string{
		`"world"`, `"descriptor"`, `"entities"`, `"predicate_schemas"`,
		`"initial_facts"`, `"records"`, `"artifact_corpus"`,
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("inference bootstrap contains world authority %q: %s", forbidden, raw)
		}
	}

	path := filepath.Join(t.TempDir(), "public-inference.json")
	if err := SealPublicInferenceBundle(path, bundle); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadPublicInferenceBundle(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Authority != bundle.Authority || loaded.Catalog.SHA256 != bundle.Catalog.SHA256 {
		t.Fatalf("loaded public inference bundle=%+v", loaded)
	}
}

func TestPublicEpisodeIdentityCannotDependOnPrivateSeedOrOracle(t *testing.T) {
	paired := validPairedRunAuthority(t)
	public, err := NewPublicRunAuthority(paired, VariantFullCognition)
	if err != nil {
		t.Fatal(err)
	}
	want, err := PublicVariantEpisodeRef(public)
	if err != nil {
		t.Fatal(err)
	}
	changed := paired
	changed.Seed++
	changed.OracleSHA256 = strings.Repeat("9", 64)
	got, err := VariantEpisodeRef(changed, VariantFullCognition)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("private authority changed public episode identity: %v != %v", got, want)
	}
}
