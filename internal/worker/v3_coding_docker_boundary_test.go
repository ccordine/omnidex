package worker

import (
	"os"
	"strings"
	"testing"
)

func TestCodingVerificationHasNoNativeExecutionFallback(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"runRecordedVerificationCommand", "runDirectCodingVerificationProcess", "directCodingVerificationProcessEnvironment", "VerificationHostCleanup"} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s retains removed native verification %s", name, forbidden)
			}
		}
	}
	for _, name := range []string{"v3_coding_typescript_stage_workspace.go", "v3_coding_typescript_dependencies.go", "v3_coding_typescript_host_verification.go", "v3_coding_go_stage_workspace.go", "v3_coding_go_host_verification.go", "v3_compiled_language_workspace.go"} {
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{`"os/exec"`, "os.MkdirTemp(", "os.RemoveAll("} {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s executes or stages a host toolchain through %s", name, forbidden)
			}
		}
	}
}
