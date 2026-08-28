package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceModelResponsesHaveNoDiscardedWrapperCompatibility(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	for _, relative := range []string{
		"internal/assemblyline",
		"internal/gofragment",
		"internal/worker",
	} {
		walkProductionSource(t, filepath.Join(root, relative), func(path, source string) {
			for _, forbidden := range []string{
				"typeScriptResponseSegments",
				"typeScriptFenceOpening",
				"trimFenceBodyEnd",
				"projectTypeScriptRepairRegionText",
				"NewSourceDeclarationPortableResultProjection",
				"directCodingTypeScriptProjectionWarning",
			} {
				if strings.Contains(source, forbidden) {
					t.Errorf("source response boundary %s retained compatibility symbol %q", path, forbidden)
				}
			}
		})
	}

	checks := map[string][]string{
		"internal/assemblyline/typescript_model_response.go": {
			"parseSingleTypeScriptFunction(\n\t\traw",
			"Source: raw",
			"DiscardedBytes: 0",
		},
		"internal/assemblyline/bounded_source_fragment.go": {
			"content := candidate",
			"NewExactSourceDeclarationPortableResultProjection(content)",
		},
		"internal/gofragment/model_response.go": {
			"raw != strings.TrimSpace(raw)",
			"Source: raw, StartByte: 0, EndByte: len(raw)",
		},
		"internal/assemblyline/portable_result.go": {
			"projection is not the complete exact response",
		},
	}
	for relative, required := range checks {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, fragment := range required {
			if !strings.Contains(source, fragment) {
				t.Errorf("exact source response boundary %s omitted %q", relative, fragment)
			}
		}
	}
}
