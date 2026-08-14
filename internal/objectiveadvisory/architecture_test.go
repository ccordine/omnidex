package objectiveadvisory

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestObjectiveAdvisoryCoreHasNoExecutionPersistenceOrProseRoutingAuthority(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve objective advisory package path")
	}
	directory := filepath.Dir(current)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		`"os/exec"`, `"path/filepath"`, `"regexp"`,
		`internal/queue`, `internal/taskstate`, `internal/worker`,
		`ReasoningAgent`, `AdvisorAgent`, `CognitionDecision`,
		`SelectOperation`, `SelectTool`, `CreateObjective`, `CompleteObjective`,
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, readErr := os.ReadFile(filepath.Join(directory, entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Fatalf("%s contains forbidden advisory authority %q", entry.Name(), token)
			}
		}
	}
}
