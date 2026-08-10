package cognitiongauntlet

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicRunAuthorityOmitsPrivateEvaluationAuthority(t *testing.T) {
	paired := validPairedRunAuthority(t)
	paired.OracleSHA256 = strings.Repeat("9", 64)
	public, err := NewPublicRunAuthority(paired, VariantFullCognition)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	assertNoPrivateEvaluationAuthority(t, encoded, paired)

	manifest := validEpisodeManifest(paired.RatGeneration, terminalTestPayload(t))
	manifest.PublicRunAuthoritySHA256, err = public.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	assertNoPrivateEvaluationAuthority(t, encoded, paired)
	if strings.Contains(string(encoded), `"run_authority_sha256":`) {
		t.Fatal("episode retained the removed full run-authority field")
	}
}

func TestPrivateAuthorityMustProjectExactlyToSealedPublicAuthority(t *testing.T) {
	paired := validPairedRunAuthority(t)
	public, err := NewPublicRunAuthority(paired, VariantFullCognition)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePublicRunAuthorityProjection(paired, public); err != nil {
		t.Fatal(err)
	}

	changed := paired
	changed.Seed++
	if err := ValidatePublicRunAuthorityProjection(changed, public); err != nil {
		t.Fatal("private seed correctly remains outside the public projection:", err)
	}
	changed = paired
	changed.OracleSHA256 = strings.Repeat("e", 64)
	if err := ValidatePublicRunAuthorityProjection(changed, public); err != nil {
		t.Fatal("private oracle identity correctly remains outside the public projection:", err)
	}
	changed = paired
	changed.Budget.ModelCalls++
	if err := ValidatePublicRunAuthorityProjection(changed, public); err == nil {
		t.Fatal("changed public budget projected to a sealed public authority")
	}
	changed = paired
	changed.Scenario.SHA256 = strings.Repeat("f", 64)
	if err := ValidatePublicRunAuthorityProjection(changed, public); err == nil {
		t.Fatal("changed public scenario projected to a sealed public authority")
	}
}

func assertNoPrivateEvaluationAuthority(
	t *testing.T,
	encoded []byte,
	paired PairedRunAuthority,
) {
	t.Helper()
	text := string(encoded)
	for _, forbidden := range []string{
		`"seed"`, `"oracle_sha256"`, `"oracle_quality"`,
		`"witness"`, `"task_archetype"`, `"case_id"`, `"suite"`,
		`"fixture_version"`, `"generator_version"`, `"difficulty"`, paired.OracleSHA256,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("public authority contains private value %q: %s", forbidden, text)
		}
	}
}
