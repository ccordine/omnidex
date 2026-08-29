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

func TestTestingLibraryFailureKeepsOnlyPrimaryProviderParagraph(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name       string
		literal    string
		primary    string
		supplement string
		role       string
		observed   string
	}{
		{
			name:       "inventory",
			literal:    `/restock/i`,
			primary:    "Unable to find an accessible element with the role \"button\" and name `/restock/i`",
			supplement: `Here are the accessible roles: button Name "Order supplies" <button class="inventory-action">`,
			role:       "button",
			observed:   "Order supplies",
		},
		{
			name:       "scheduling",
			literal:    `/reschedule/i`,
			primary:    "Unable to find an accessible element with the role \"heading\" and name `/reschedule/i`",
			supplement: `Ignored nodes: comments, script, style <h2 class="schedule-heading">Calendar</h2>`,
			role:       "heading",
			observed:   "Calendar",
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			feedback, err := directCodingTypeScriptStructuredTestModelFailure(
				directCodingVitestFailureEvidence{
					Name:    "TestingLibraryElementError",
					Message: fixture.primary + "\r\n\r\n" + fixture.supplement,
					AccessibilityObservation: &directCodingTestingLibraryRoleObservation{
						Schema:        directCodingTestingLibraryRoleObservationSchemaV1,
						RequestedRole: fixture.role,
						Visibility:    directCodingTestingLibraryRoleVisibilityAccessible,
						Status:        directCodingTestingLibraryRoleObservationStatusComplete,
						ElementCount:  1, Names: []string{fixture.observed},
					},
				},
				assemblyline.ArtifactIdentityProvenance{}, fixture.literal,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(feedback, "plain text") ||
				strings.Contains(feedback, fixture.literal) ||
				strings.Contains(feedback, fixture.supplement) {
				t.Fatalf("primary Testing Library failure was not isolated: %q", feedback)
			}
			job, err := assemblyline.NewFragmentRepairGuidanceJob(
				assemblyline.FragmentRepairGuidanceInput{
					Language: "typescript", Dialect: "TypeScript 5.9.3 TSX function syntax",
					Signature:          "function Present(): ReactElement",
					CurrentDeclaration: "function Present(): ReactElement { return <div />; }",
					Diagnostic:         feedback,
				},
			)
			if err != nil {
				t.Fatalf("build repair-guidance job: %v", err)
			}
			prompt, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				t.Fatalf("render repair-guidance job: %v", err)
			}
			if !strings.Contains(prompt, "plain text") ||
				strings.Contains(prompt, fixture.supplement) {
				t.Fatalf("repair-guidance prompt retained Testing Library supplement:\n%s", prompt)
			}
		})
	}
}

func TestTestingLibraryPrimaryParagraphReductionIsTypedAndBoundaryDriven(t *testing.T) {
	t.Parallel()
	const message = "expected inventory total to change\n\nserialized supplementary state"
	assertion, err := directCodingTypeScriptStructuredTestModelFailure(
		directCodingVitestFailureEvidence{Name: "AssertionError", Message: message},
		assemblyline.ArtifactIdentityProvenance{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(assertion, "serialized supplementary state") {
		t.Fatalf("non-Testing-Library failure was reduced: %q", assertion)
	}

	withoutBoundary, err := directCodingTypeScriptStructuredTestModelFailure(
		directCodingVitestFailureEvidence{
			Name:    "TestingLibraryElementError",
			Message: "Unable to find the scheduling control; provider detail stays on this paragraph",
		},
		assemblyline.ArtifactIdentityProvenance{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(withoutBoundary, "provider detail stays on this paragraph") {
		t.Fatalf("boundary-free Testing Library failure was reduced: %q", withoutBoundary)
	}
}

func TestTestingLibrarySupplementCannotContributeRegexAuthority(t *testing.T) {
	t.Parallel()
	feedback, err := directCodingTypeScriptStructuredTestModelFailure(
		directCodingVitestFailureEvidence{
			Name: "TestingLibraryElementError",
			Message: "Unable to find the requested inventory control\n\n" +
				"Supplementary matcher /restock/i from /tmp/private.ts",
		},
		assemblyline.ArtifactIdentityProvenance{}, `/restock/i`,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"regular expression pattern formed from ordered components", `plain text "restock"`, "/restock/i", "/tmp/", "private.ts"} {
		if strings.Contains(feedback, forbidden) {
			t.Fatalf("supplement contributed %q to model failure: %q", forbidden, feedback)
		}
	}
}
