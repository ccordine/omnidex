package assemblyline

import (
	"strings"
	"testing"
)

func TestUnspecifiedApplicationSurfaceUsesCodeOwnedBrowserDefault(t *testing.T) {
	t.Parallel()
	fixtures := []string{
		"Build a tool that tracks recurring maintenance dates.",
		"Create software that turns a paragraph into a word-frequency summary.",
	}
	for _, request := range fixtures {
		request := request
		t.Run(request, func(t *testing.T) {
			t.Parallel()
			input := ApplicationClassificationInput{UserRequest: request}
			prompt, err := BuildApplicationClassificationPrompt(input)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range []string{
				"The request does not constrain its observable delivery surface.",
				"A missing surface constraint is different from an explicit requirement outside the registered set",
				"Answer with A or B or C or D.",
			} {
				if !strings.Contains(prompt, required) {
					t.Fatalf("surface prompt omitted %q: %s", required, prompt)
				}
			}
			for _, forbidden := range []string{
				string(ApplicationSurfaceBrowser),
				string(ApplicationSurfaceCommandLine),
				string(ApplicationSurfaceUnspecified),
				string(ApplicationSurfaceUnsupported),
			} {
				if strings.Contains(prompt, forbidden) {
					t.Fatalf("surface prompt exposed code-owned value %q: %s", forbidden, prompt)
				}
			}
			classification, err := DecodeApplicationClassification(
				input, "C",
			)
			if err != nil {
				t.Fatal(err)
			}
			if classification.Schema != ApplicationClassificationSchemaV2 ||
				classification.Surface != ApplicationSurfaceUnspecified {
				t.Fatalf("classification=%+v", classification)
			}
			if _, err := DecodeApplicationClassification(
				input,
				string(ApplicationSurfaceUnspecified),
			); err == nil {
				t.Fatal("code-owned surface value was accepted as model output")
			}
			surface, err := ResolveApplicationSurface(classification)
			if err != nil || surface != ApplicationDefaultSurface ||
				ApplicationDefaultSurface != ApplicationSurfaceBrowser {
				t.Fatalf("resolved surface=%q error=%v", surface, err)
			}
		})
	}
}

func TestExplicitUnsupportedApplicationSurfaceDoesNotUseDefault(t *testing.T) {
	t.Parallel()
	input := ApplicationClassificationInput{
		UserRequest: "Build a native smartwatch application.",
	}
	classification, err := DecodeApplicationClassification(
		input, "D",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveApplicationSurface(classification); err == nil ||
		!strings.Contains(err.Error(), "requires an unsupported delivery surface") {
		t.Fatalf("unsupported surface resolution error=%v", err)
	}
}

func TestUnresolvedApplicationSurfaceCannotCompile(t *testing.T) {
	t.Parallel()
	specification := ApplicationSpecification{
		Surface:      ApplicationSurfaceUnspecified,
		ProductQuote: "a generic utility",
		Requirements: []Requirement{{
			ID: "requirement_001", SourceQuote: "The software returns a summary.",
		}},
	}
	if err := specification.Validate(); err == nil ||
		!strings.Contains(err.Error(), "must be resolved before compilation") {
		t.Fatalf("unresolved specification error=%v", err)
	}
}

func TestSupersededApplicationClassificationSchemaIsRejected(t *testing.T) {
	t.Parallel()
	classification := ApplicationClassification{
		Schema: "omnidex.application-class.v1", Surface: ApplicationSurfaceBrowser,
	}
	if err := classification.Validate(); err == nil ||
		!strings.Contains(err.Error(), ApplicationClassificationSchemaV2) {
		t.Fatalf("superseded classification schema error=%v", err)
	}
}
