package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestArtifactAdapterRegistryProvidesTreeStackAndClassifiesLeaves(t *testing.T) {
	context, err := directCodingTreeTechnicalContext(assemblyline.ApplicationSurfaceBrowser)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(context, "TypeScript React") || !strings.Contains(context, ".test.tsx") {
		t.Fatalf("technical context=%q", context)
	}
	adapter, kind, err := directCodingArtifactAdapterForTreePath(assemblyline.ApplicationSurfaceBrowser, "tests/counter.test.tsx")
	if err != nil {
		t.Fatal(err)
	}
	if adapter.ID != genericTypeScriptBrowserAdapter || kind != assemblyline.TargetArtifactVerification {
		t.Fatalf("adapter=%+v kind=%q", adapter, kind)
	}
	if _, _, err := directCodingArtifactAdapterForTreePath(assemblyline.ApplicationSurfaceBrowser, "src/main/java/Counter.java"); err == nil || !strings.Contains(err.Error(), "no registered artifact adapter") {
		t.Fatalf("java adapter error=%v", err)
	}
}
