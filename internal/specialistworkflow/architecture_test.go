package specialistworkflow_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestKernelContainsOnlyTypedLifecycleLaws(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Clean(entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, forbidden := range []string{
			"internal/llm", "internal/tools", "internal/worker", "internal/artifacts",
			"internal/web", "internal/browser", "encoding/json", "reflect", "map[",
			"type Gap ", "type Station ", "type Candidate ", "type Tool ",
			"type Action ", "type Step ", "type Fact ", "type Artifact ", "type Evidence ",
		} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s contains forbidden generic/domain surface %q", entry.Name(), forbidden)
			}
		}
		if regexp.MustCompile(`\bany\b`).MatchString(source) {
			t.Errorf("%s contains untyped any authority", entry.Name())
		}
		if lines := strings.Count(source, "\n") + 1; lines >= 300 {
			t.Errorf("%s has %d lines; kernel files must stay below 300", entry.Name(), lines)
		}
	}
}
