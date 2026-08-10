package cognitionenv

import (
	"reflect"
	"strings"
	"testing"

	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestInitialObservationBindsExactPathBlindNeed(t *testing.T) {
	base, analysis, snapshot := testInvestigation(t, repositoryretrieval.OperationSemanticExcerpts)
	firstNeed, err := NewNeedAuthority("Determine which declaration owns bounded retry accounting.")
	if err != nil {
		t.Fatal(err)
	}
	secondNeed, err := NewNeedAuthority("Determine which declaration validates current attempt authority.")
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewInvestigation(41, snapshot, analysis, firstNeed, base.operation, base.query)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewInvestigation(41, snapshot, analysis, secondNeed, base.operation, base.query)
	if err != nil {
		t.Fatal(err)
	}
	firstEnvironment, _, _ := testEnvironment(t, first, &recordingBuilder{})
	secondEnvironment, _, _ := testEnvironment(t, second, &recordingBuilder{})
	firstStart, err := firstEnvironment.Start(t.Context(), first.Ref())
	if err != nil {
		t.Fatal(err)
	}
	replay, err := firstEnvironment.Start(t.Context(), first.Ref())
	if err != nil {
		t.Fatal(err)
	}
	secondStart, err := secondEnvironment.Start(t.Context(), second.Ref())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstStart, replay) {
		t.Fatal("same accepted need did not produce one stable initial observation")
	}
	if firstStart.Observations[0].Content == secondStart.Observations[0].Content ||
		!strings.Contains(firstStart.Observations[0].Content, firstNeed.Content) ||
		!strings.Contains(secondStart.Observations[0].Content, secondNeed.Content) {
		t.Fatal("different accepted needs did not remain distinct model-visible authority")
	}

	if _, err := NewNeedAuthority("Inspect sample.go for the target declaration."); err == nil {
		t.Fatal("NewNeedAuthority accepted a generic file identity")
	}
	exactIdentityContent := "Inspect repository identity " + snapshot.Files[0].ID
	exactIdentityDigest := textDigest(exactIdentityContent)
	prohibited := NeedAuthority{
		ID:      "repository-need-" + exactIdentityDigest,
		Content: exactIdentityContent, ContentSHA256: exactIdentityDigest,
	}
	if _, err := NewInvestigation(
		41, snapshot, analysis, prohibited, base.operation, base.query,
	); err == nil || !strings.Contains(err.Error(), "prohibited path") {
		t.Fatalf("path-bearing need error=%v", err)
	}
	for _, pathBearing := range []string{
		"Inspect /tmp/unrelated for the declaration.",
		"Inspect other.yaml for the declaration.",
		`Inspect C:\\work\\unrelated for the declaration.`,
	} {
		if _, err := NewNeedAuthority(pathBearing); err == nil {
			t.Fatalf("NewNeedAuthority accepted path identity %q", pathBearing)
		}
	}
}
