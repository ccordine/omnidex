package omni

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOllamaBackendProfileRejectsExternalBackendDropins(t *testing.T) {
	root := repoRootFromOmniTest(t)
	dropins := t.TempDir()
	writeFixtureFile(t, filepath.Join(dropins, "override.conf"), "[Service]\nEnvironment=\"OLLAMA_HOST=0.0.0.0:11434\"\n", 0o600)
	assertOllamaBackendDropins(t, root, dropins, true, "")

	conflict := filepath.Join(dropins, "gpu.conf")
	writeFixtureFile(t, conflict, "[Service]\nEnvironment=\"OLLAMA_LLM_LIBRARY=rocm\"\n", 0o600)
	assertOllamaBackendDropins(t, root, dropins, false, conflict)
}

func TestEveryOllamaProfileChecksExternalDropinsBeforeWriting(t *testing.T) {
	root := repoRootFromOmniTest(t)
	for _, path := range []string{
		"scripts/ollama-stable-cpu.sh",
		"scripts/ollama-rx7700s-rocm.sh",
		"scripts/ollama-vulkan.sh",
	} {
		body := readRepoScript(t, root, path)
		check := strings.Index(body, "ollama_require_no_external_backend_dropins")
		write := strings.Index(body, "cat > \"${dropin_file}\"")
		if check < 0 || write < 0 || check >= write {
			t.Fatalf("%s does not reject external backend authority before writing", path)
		}
	}
}

func TestOllamaDropinConsolidationArchivesExactConflictsAndPreservesHostSettings(t *testing.T) {
	root := repoRootFromOmniTest(t)
	dropins := t.TempDir()
	archive := filepath.Join(dropins, "legacy-fixture")
	override := "[Service]\nEnvironment=\"OLLAMA_HOST=0.0.0.0:11434\"\nEnvironment=\"HSA_OVERRIDE_GFX_VERSION=11.0.0\"\n"
	gpu := "[Service]\nEnvironment=\"OLLAMA_LLM_LIBRARY=rocm\"\nEnvironment=\"ROCR_VISIBLE_DEVICES=0\"\n"
	writeFixtureFile(t, filepath.Join(dropins, "override.conf"), override, 0o600)
	writeFixtureFile(t, filepath.Join(dropins, "gpu.conf"), gpu, 0o600)
	writeFixtureFile(t, filepath.Join(dropins, "zz-omni-vulkan.conf"), "[Service]\nEnvironment=\"OLLAMA_VULKAN=1\"\n", 0o600)

	script := `
set -euo pipefail
source "$1/scripts/ollama-profile-lib.sh"
ollama_require_one_omni_backend_profile "$2"
ollama_archive_external_backend_dropins "$2" "$3"
ollama_require_no_external_backend_dropins "$2"
`
	command := exec.Command("bash", "-c", script, "ollama-dropin-consolidation", root, dropins, archive)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("consolidate drop-ins: %v: %s", err, output)
	}
	if got := readFixtureFile(t, filepath.Join(archive, "override.conf")); got != override {
		t.Fatalf("archived override changed: %q", got)
	}
	if got := readFixtureFile(t, filepath.Join(archive, "gpu.conf")); got != gpu {
		t.Fatalf("archived GPU profile changed: %q", got)
	}
	if got := readFixtureFile(t, filepath.Join(dropins, "override.conf")); got != "[Service]\nEnvironment=\"OLLAMA_HOST=0.0.0.0:11434\"\n" {
		t.Fatalf("host-only override = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dropins, "gpu.conf")); !os.IsNotExist(err) {
		t.Fatalf("backend-only external profile remains active: %v", err)
	}
}

func TestOllamaConsolidationScriptUsesTheAuditedArchiveBoundary(t *testing.T) {
	root := repoRootFromOmniTest(t)
	body := readRepoScript(t, root, "scripts/ollama-consolidate-dropins.sh")
	for _, required := range []string{
		"ollama_require_one_omni_backend_profile",
		"ollama_archive_external_backend_dropins",
		"ollama_require_no_external_backend_dropins",
		"systemctl daemon-reload",
		"systemctl restart ollama",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("consolidation script omits %q", required)
		}
	}
}

func assertOllamaBackendDropins(t *testing.T, root, dropins string, wantSuccess bool, want string) {
	t.Helper()
	script := `
set -euo pipefail
source "$1/scripts/ollama-profile-lib.sh"
ollama_require_no_external_backend_dropins "$2"
`
	command := exec.Command("bash", "-c", script, "ollama-profile-exclusivity", root, dropins)
	command.Env = os.Environ()
	output, err := command.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("host-only drop-in rejected: %v: %s", err, output)
	}
	if !wantSuccess && (err == nil || !strings.Contains(string(output), want)) {
		t.Fatalf("external backend conflict error=%v output=%q want=%q", err, output, want)
	}
}
