package qwenselector

import (
	"os"
	"strings"
	"testing"
)

func TestSelectorHasNoAgentRuntimeOrPersistenceDependency(t *testing.T) {
	t.Parallel()
	for _, file := range []string{"envelope.go", "selector.go"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, forbidden := range []string{
			"/cognitionpolicy", "/cognitionruntime", "/store", "/queue", "/postgres",
			"database/sql", "pgx", "CognitionDecision", "OperationID", "ActionSchema",
		} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s contains forbidden dependency or universal-agent concept %q", file, forbidden)
			}
		}
	}
}
