package envfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadAcceptsOneExactValuePerKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("# managed\nCORE_URL=https://omni.example\nWORKER_COUNT=3\nEMPTY=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 3 || values["CORE_URL"] != "https://omni.example" ||
		values["WORKER_COUNT"] != "3" || values["EMPTY"] != "" {
		t.Fatalf("values = %#v", values)
	}
}

func TestReadRejectsAmbiguousOrExecutableSyntax(t *testing.T) {
	tests := map[string]string{
		"duplicate":   "A=1\nA=2\n",
		"export":      "export A=1\n",
		"substitute":  "A=$(id)\n",
		"invalid key": "lower=1\n",
		"nul":         "A=x\x00y\n",
	}
	for name, contents := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := Read(path)
			if err == nil {
				t.Fatalf("accepted %q", contents)
			}
		})
	}
}

func TestReadRejectsOversizedEnvironmentBeforeParsing(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("A="+strings.Repeat("x", MaxBytes)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Read(path)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v", err)
	}
}
