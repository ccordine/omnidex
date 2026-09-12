package version

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalBuildUsesCurrentPackagesWithoutGitProvenanceGates(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "scripts", "build-core.sh")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"managed_checkout", "OMNIDEX_COMMIT", "verify_binary_commit",
		"./cmd/core", "./cmd/cli", "rm -f",
	} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("local build still contains obsolete gate %q", forbidden)
		}
	}
	for _, required := range []string{"./cmd/omnidex", "./cmd/omni", "go build", "-buildvcs=false"} {
		if !strings.Contains(string(source), required) {
			t.Fatalf("local build is missing %q", required)
		}
	}
	for _, obsolete := range []string{"./cmd/core", "./cmd/cli"} {
		output, err := exec.Command("bash", path, "--package", obsolete).CombinedOutput()
		if err == nil || !strings.Contains(string(output), "unsupported Omnidex binary package") {
			t.Fatalf("obsolete package %q did not fail explicitly: %s (%v)", obsolete, output, err)
		}
	}
}
