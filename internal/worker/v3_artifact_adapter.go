package worker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// directCodingArtifactAdapter is deterministic language/configuration support
// for one artifact class. It does not expose tools or grant any authority to a
// model. Each adapter owns recognition and the deterministic machinery that
// makes its leaf jobs verifiable.
type directCodingArtifactAdapter struct {
	ID              string
	Surface         assemblyline.ApplicationSurface
	TreeDescription string
	Recognize       func(path string) (assemblyline.TargetArtifactKind, bool)
}

func registeredDirectCodingArtifactAdapters() []directCodingArtifactAdapter {
	return []directCodingArtifactAdapter{
		{
			ID:              genericTypeScriptBrowserAdapter,
			Surface:         assemblyline.ApplicationSurfaceBrowser,
			TreeDescription: "TypeScript React workload source (.tsx) and browser-test (.test.tsx) files",
			Recognize: func(path string) (assemblyline.TargetArtifactKind, bool) {
				switch {
				case strings.HasSuffix(path, ".test.tsx"):
					return assemblyline.TargetArtifactVerification, true
				case strings.HasSuffix(path, ".tsx"):
					return assemblyline.TargetArtifactImplementation, true
				default:
					return "", false
				}
			},
		},
	}
}

func directCodingTreeTechnicalContext(surface assemblyline.ApplicationSurface) (string, error) {
	descriptions := make([]string, 0)
	for _, adapter := range registeredDirectCodingArtifactAdapters() {
		if adapter.Surface == surface {
			descriptions = append(descriptions, adapter.TreeDescription)
		}
	}
	if len(descriptions) == 0 {
		return "", fmt.Errorf("no registered artifact adapter supports application surface %s", surface)
	}
	sort.Strings(descriptions)
	return "Registered code-owned artifact formats for this surface: " + strings.Join(descriptions, "; ") + ". Return only workload-specific paths in one registered format. Registered adapters independently supply any runtime, shell, bootstrap, manifests, styles, and their tests.", nil
}

func directCodingArtifactAdapterForTreePath(
	surface assemblyline.ApplicationSurface,
	path string,
) (directCodingArtifactAdapter, assemblyline.TargetArtifactKind, error) {
	for _, adapter := range registeredDirectCodingArtifactAdapters() {
		if adapter.Surface != surface {
			continue
		}
		if kind, recognized := adapter.Recognize(path); recognized {
			return adapter, kind, nil
		}
	}
	return directCodingArtifactAdapter{}, "", fmt.Errorf(
		"target-tree file %q has no registered artifact adapter for surface %s", path, surface,
	)
}
