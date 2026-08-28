package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationClassificationIsOneRawRegisteredSurface(t *testing.T) {
	t.Parallel()
	input := ApplicationClassificationInput{UserRequest: "Build a browser application."}
	job, err := NewApplicationClassificationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		string(ApplicationSurfaceBrowser), string(ApplicationSurfaceCommandLine),
		string(ApplicationSurfaceService), string(ApplicationSurfaceUnsupported),
		"no JSON, quotes, label, Markdown, or commentary",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("classification prompt omitted %q: %s", required, prompt)
		}
	}
	classification, err := DecodeApplicationClassification(input, string(ApplicationSurfaceBrowser))
	if err != nil || classification.Schema != ApplicationClassificationSchemaV1 ||
		classification.Surface != ApplicationSurfaceBrowser {
		t.Fatalf("classification=%+v err=%v", classification, err)
	}
	for _, raw := range []string{
		`{"surface":"browser_application"}`,
		`"browser_application"`,
		"surface: browser_application",
		"desktop_application",
	} {
		if _, err := DecodeApplicationClassification(input, raw); err == nil {
			t.Fatalf("invalid raw classification accepted: %q", raw)
		}
	}
}
