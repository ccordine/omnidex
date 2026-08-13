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

func TestArtifactRedactionDoesNotTreatDottedGoSymbolsAsPaths(t *testing.T) {
	t.Parallel()
	request := "Use http.Client and time.Time while leaving transport.go unchanged."
	redacted, identities, err := RedactArtifactIdentities(request)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(redacted, "http.Client") || !strings.Contains(redacted, "time.Time") {
		t.Fatalf("dotted symbols were corrupted: %q", redacted)
	}
	if len(identities) != 1 || identities[0].Value != "transport.go" {
		t.Fatalf("artifact identities=%#v", identities)
	}
}
