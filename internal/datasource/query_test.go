package datasource

import (
	"os"
	"strings"
	"testing"
)

func TestConnectReadOnlyDoesNotRelyOnOnePooledSessionSetting(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(source), "SET default_transaction_read_only") {
			t.Fatalf("%s relies on a setting applied to one pooled session", entry.Name())
		}
	}
}
