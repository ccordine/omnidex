package version

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildAndDeploymentScriptsDoNotRequireProvenanceReceipts(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..")
	for _, path := range []string{
		"scripts/build-release.sh", "scripts/build-release-lib.sh",
		"scripts/managed-release-install-lib.sh", "scripts/managed-checkout-lib.sh",
		"scripts/update-runtime-lib.sh", "scripts/compose-deployment.sh",
		"install.sh", "install-release.sh", "update.sh", "docker-rebuild.sh",
		"Dockerfile", "docker-compose.yml",
	} {
		t.Run(path, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(root, path))
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{
				"OMNIDEX_COMMIT", "RELEASE_COMMIT", "SHA256SUMS", "--expect-commit",
				"verify_binary_commit", "release-binary-identity", "SourceSHA256",
			} {
				if strings.Contains(string(source), forbidden) {
					t.Errorf("%s contains retired provenance gate %q", path, forbidden)
				}
			}
			if strings.HasSuffix(path, ".sh") {
				if output, err := exec.Command("bash", "-n", filepath.Join(root, path)).CombinedOutput(); err != nil {
					t.Fatalf("shell syntax: %s (%v)", output, err)
				}
			}
		})
	}
	for _, path := range []string{"scripts/build-release.sh", "scripts/managed-checkout-lib.sh"} {
		source, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(source), "/build-core.sh") || strings.Contains(string(source), "go build") {
			t.Errorf("%s must use the one shared binary builder", path)
		}
	}
}

func TestReleaseInputValidation(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, version, codename, targets, dist, wantErr string
	}{
		{"native layout", "v0.5.0", "Charmeleon", "linux/amd64", "release", ""},
		{"invalid version", "../release", "Charmeleon", "linux/amd64", "release", "invalid release version"},
		{"invalid codename", "v0.5.0", "bad name", "linux/amd64", "release", "invalid release codename"},
		{"unknown target", "v0.5.0", "Charmeleon", "plan9/amd64", "release", "unsupported release target"},
		{"duplicate target", "v0.5.0", "Charmeleon", "linux/amd64 linux/amd64", "release", "duplicate release target"},
		{"root destination", "v0.5.0", "Charmeleon", "linux/amd64", "/", "explicit and non-root"},
		{"repository destination", "v0.5.0", "Charmeleon", "linux/amd64", root, "repository root"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dist := test.dist
			if dist == "release" {
				dist = filepath.Join(t.TempDir(), dist)
			}
			output, err := exec.Command("bash", "-eu", "-c", `
source "$1/scripts/build-release.sh"
VERSION="$2"
CODENAME="$3"
read -r -a TARGETS <<< "$4"
DIST_DIR="$5"
validate_release_inputs
validate_dist_dir
`, "release-test", root, test.version, test.codename, test.targets, dist).CombinedOutput()
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("valid release inputs failed: %s (%v)", output, err)
				}
			} else if err == nil || !strings.Contains(string(output), test.wantErr) {
				t.Fatalf("error = %v, output = %s, want %q", err, output, test.wantErr)
			}
		})
	}
}

func TestReleaseRefusesExistingPublicationBeforeBuilding(t *testing.T) {
	t.Parallel()
	dist := t.TempDir()
	publication := filepath.Join(dist, "omnidex-v0.5.0")
	if err := os.Mkdir(publication, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("bash", "-eu", "-c", `
source "$1/scripts/build-release.sh"
validate_release_cgo_targets() { :; }
build_target() { printf '%s\n' 'unexpected build invocation'; exit 99; }
main --dist "$2" --target linux/amd64
`, "release-test", root, dist).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "release already exists") ||
		strings.Contains(string(output), "unexpected build invocation") {
		t.Fatalf("existing release was not rejected before build: %s (%v)", output, err)
	}
	entries, err := os.ReadDir(dist)
	if err != nil || len(entries) != 1 || entries[0].Name() != "omnidex-v0.5.0" {
		t.Fatalf("publication changed: entries = %v, error = %v", entries, err)
	}
}

func TestVersionMetadataDoesNotContainAttestationFields(t *testing.T) {
	t.Parallel()
	metadata := JSON()
	for _, key := range []string{"commit", "source_sha256"} {
		if _, exists := metadata[key]; exists {
			t.Errorf("version metadata retains %q", key)
		}
	}
	if metadata["version"] == "" || metadata["codename"] == "" {
		t.Fatal("release labels are missing")
	}
}
