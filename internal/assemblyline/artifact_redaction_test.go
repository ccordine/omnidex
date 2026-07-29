package assemblyline

import (
	"strings"
	"testing"
)

func TestArtifactRedactionPreservesVersionsAndHidesSourceIdentity(t *testing.T) {
	redacted, identities, err := RedactArtifactIdentities("Build this in Go 1.22. Do not modify REQUEST.md or docs/current-plan.md.")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(redacted, "Go 1.22") {
		t.Fatalf("version was corrupted: %q", redacted)
	}
	for _, leaked := range []string{"REQUEST.md", "docs/current-plan.md"} {
		if strings.Contains(redacted, leaked) {
			t.Fatalf("identity %q leaked: %q", leaked, redacted)
		}
	}
	if len(identities) != 2 || !strings.Contains(redacted, identities[0].Token) || !strings.Contains(redacted, identities[1].Token) {
		t.Fatalf("redacted=%q identities=%#v", redacted, identities)
	}
}
