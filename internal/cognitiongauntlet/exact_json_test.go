package cognitiongauntlet

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/labyrinth"
)

func TestStrictGauntletDecoderRejectsRecursiveDuplicatesAndCaseAliases(t *testing.T) {
	t.Parallel()
	type child struct {
		ID string `json:"id"`
	}
	type envelope struct {
		Children []child `json:"children"`
	}
	for name, raw := range map[string]string{
		"nested duplicate":  `{"children":[{"id":"one","id":"two"}]}`,
		"nested case alias": `{"children":[{"ID":"one"}]}`,
		"trailing object":   `{"children":[]} {}`,
	} {
		var target envelope
		if err := decodeStrictJSON([]byte(raw), &target, name); err == nil {
			t.Fatalf("decoder accepted %s", name)
		}
	}
}

func TestGenericCausalResultRejectsDuplicateEvidenceKeys(t *testing.T) {
	t.Parallel()
	identity := labyrinth.EvidenceIdentity{ID: "evidence-record", SHA256: strings.Repeat("a", 64)}
	raw := []byte(`{"records":[{"id":"` + identity.ID + `","id":"other","content_sha256":"` + identity.SHA256 + `"}]}`)
	if _, _, err := countEvidenceIdentities(raw, identity); err == nil {
		t.Fatal("causal evidence result accepted a duplicate identity key")
	}
}
