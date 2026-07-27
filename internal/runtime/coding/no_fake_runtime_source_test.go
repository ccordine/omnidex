package coding

import (
	"os"
	"strings"
	"testing"
)

func TestCodingRuntimeContainsNoSyntheticExecutorOrPassValidator(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"NewDeterministicEngine", "noopCoder", "noopTestWriter", "passValidator", "deterministicSummarizer"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("%s retains forbidden fake coding runtime %q", entry.Name(), forbidden)
			}
		}
	}
}
