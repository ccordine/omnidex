package worker

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrdinalServiceTechnicalProjectorsAreAbsent(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat("v3_service_technical_policy.go"); !os.IsNotExist(err) {
		t.Fatalf("obsolete service technical-policy source remains: %v", err)
	}
	forbidden := []string{
		"ProjectServiceEndpoint",
		"ProjectServiceStateInterface",
		"projectPHPServiceEndpoint",
		"projectPHPServiceStateInterface",
		"endpointSequence",
		"service endpoint task sequence does not match frozen workload authority",
	}
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, value := range forbidden {
			if strings.Contains(string(raw), value) {
				t.Errorf("production source %s retains ordinal technical-policy authority %q", path, value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
