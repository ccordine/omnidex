package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

func TestStructuredTypeScriptTestFailurePreservesExactProblem(t *testing.T) {
	t.Parallel()
	feedback, err := directCodingTypeScriptStructuredTestModelFailure(
		directCodingVitestFailureEvidence{
			Name:    "TypeError",
			Message: "Cannot read properties of undefined (reading 'includes')",
		},
		assemblyline.ArtifactIdentityProvenance{},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := "TypeError: Cannot read properties of undefined (reading 'includes')"
	if feedback != want {
		t.Fatalf("structured failure=%q want=%q", feedback, want)
	}
}

func TestStructuredTypeScriptTestFailureRedactsUnsafeIdentityWithoutDiscardingProblem(t *testing.T) {
	t.Parallel()
	feedback, err := directCodingTypeScriptStructuredTestModelFailure(
		directCodingVitestFailureEvidence{
			Name: "TestingLibraryElementError",
			Message: `Unable to find an element with the text matcher from /tmp/stage/src/view.test.tsx:12:3: ` +
				`(value) => value.includes('\\d+')`,
		},
		assemblyline.ArtifactIdentityProvenance{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(feedback, "TestingLibraryElementError: Unable to find an element") {
		t.Fatalf("structured failure discarded the exact problem: %q", feedback)
	}
	for _, forbidden := range []string{"/tmp/", "view.test.tsx", `\\d+`} {
		if strings.Contains(feedback, forbidden) {
			t.Fatalf("structured failure leaked %q: %q", forbidden, feedback)
		}
	}
	if modelcontext.ContainsPathIdentity(feedback) {
		t.Fatalf("structured failure retained path identity: %q", feedback)
	}
}

func TestStructuredTypeScriptTestFailureRequiresExactNameAndMessage(t *testing.T) {
	t.Parallel()
	for _, failure := range []directCodingVitestFailureEvidence{
		{Message: "observed failure"},
		{Name: "TypeError"},
	} {
		if _, err := directCodingTypeScriptStructuredTestModelFailure(
			failure, assemblyline.ArtifactIdentityProvenance{},
		); err == nil {
			t.Fatalf("accepted incomplete structured failure: %+v", failure)
		}
	}
}
