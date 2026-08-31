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
				"unspecified only when the request does not constrain",
				"Do not choose unsupported merely because the surface is omitted",
				"browser_application | command_line_application | unspecified | unsupported",
			} {
				if !strings.Contains(prompt, required) {
					t.Fatalf("surface prompt omitted %q: %s", required, prompt)
				}
			}
			classification, err := DecodeApplicationClassification(
				input, string(ApplicationSurfaceUnspecified),
			)
			if err != nil {
				t.Fatal(err)
			}
			if classification.Schema != ApplicationClassificationSchemaV2 ||
				classification.Surface != ApplicationSurfaceUnspecified {
				t.Fatalf("classification=%+v", classification)
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
		input, string(ApplicationSurfaceUnsupported),
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
