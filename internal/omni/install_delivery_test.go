package omni

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedInstallIsCompleteUpdateableAndBuildFailureAtomic(t *testing.T) {
	fixture := newManagedInstallFixture(t)
	runManagedScript(t, fixture, filepath.Join(fixture.source, "install.sh"), nil,
		"--prefix", fixture.prefix, "--skip-deps", "--yes")

	assertManagedCheckoutClean(t, fixture.prefix)
	oldHead := runFixtureGit(t, fixture.prefix, "rev-parse", "HEAD")
	oldBinary := readFixtureFile(t, filepath.Join(fixture.prefix, "bin", "omni"))
	writeFixtureFile(t, filepath.Join(fixture.prefix, ".env"), "USER_SETTING=preserved\n", 0o600)

	writeFixtureFile(t, filepath.Join(fixture.source, "README.md"), "second revision\n", 0o644)
	runFixtureGit(t, fixture.source, "add", "README.md")
	runFixtureGit(t, fixture.source, "commit", "-m", "second")
	runFixtureGit(t, fixture.source, "push", "origin", "main")
	newHead := runFixtureGit(t, fixture.source, "rev-parse", "HEAD")

	for _, failure := range []string{"OMNI_FIXTURE_NPM_FAIL", "OMNI_FIXTURE_GO_FAIL"} {
		output, err := runManagedScriptResult(fixture, filepath.Join(fixture.prefix, "update.sh"),
			[]string{failure + "=1"}, "--host-only", "--no-host-restart")
		if err == nil {
			t.Fatalf("update succeeded with %s; output=%s", failure, output)
		}
		if got := runFixtureGit(t, fixture.prefix, "rev-parse", "HEAD"); got != oldHead {
			t.Fatalf("failed update changed active HEAD from %s to %s", oldHead, got)
		}
		if got := readFixtureFile(t, filepath.Join(fixture.prefix, "bin", "omni")); got != oldBinary {
			t.Fatal("failed update replaced the active omni binary")
		}
		if got := readFixtureFile(t, filepath.Join(fixture.prefix, ".env")); got != "USER_SETTING=preserved\n" {
			t.Fatalf("failed update changed .env: %q", got)
		}
	}

	runManagedScript(t, fixture, filepath.Join(fixture.prefix, "update.sh"), nil,
		"--host-only", "--no-host-restart")
	if got := runFixtureGit(t, fixture.prefix, "rev-parse", "HEAD"); got != newHead {
		t.Fatalf("updated HEAD=%s want=%s", got, newHead)
	}
	if got := readFixtureFile(t, filepath.Join(fixture.prefix, ".env")); got != "USER_SETTING=preserved\n" {
		t.Fatalf("successful update did not preserve .env: %q", got)
	}
	assertManagedCheckoutClean(t, fixture.prefix)
}

func TestManagedUpdateRejectsIncompatiblePreservedEnvironmentBeforePublish(t *testing.T) {
	fixture := newManagedInstallFixture(t)
	runManagedScript(t, fixture, filepath.Join(fixture.source, "install.sh"), nil,
		"--prefix", fixture.prefix, "--skip-deps", "--yes")
	oldHead := runFixtureGit(t, fixture.prefix, "rev-parse", "HEAD")
	writeFixtureFile(t, filepath.Join(fixture.prefix, ".env"), "APP_ENV=production\n", 0o600)

	writeFixtureFile(t, filepath.Join(fixture.source, "README.md"), "incompatible revision\n", 0o644)
	runFixtureGit(t, fixture.source, "add", "README.md")
	runFixtureGit(t, fixture.source, "commit", "-m", "incompatible")
	runFixtureGit(t, fixture.source, "push", "origin", "main")

	output, err := runManagedScriptResult(fixture, filepath.Join(fixture.prefix, "update.sh"), nil,
		"--host-only", "--no-host-restart")
	if err == nil || !strings.Contains(output, "staged .env is incompatible") {
		t.Fatalf("incompatible update error=%v output=%s", err, output)
	}
	if got := runFixtureGit(t, fixture.prefix, "rev-parse", "HEAD"); got != oldHead {
		t.Fatalf("incompatible environment published HEAD %s, want %s", got, oldHead)
	}
	if got := readFixtureFile(t, filepath.Join(fixture.prefix, ".env")); got != "APP_ENV=production\n" {
		t.Fatalf("failed validation changed active environment: %q", got)
	}
}

func TestBuildUIFailsLoudlyWhenNPMIsMissing(t *testing.T) {
	root := repoRootFromOmniTest(t)
	path := t.TempDir()
	writeFixtureFile(t, filepath.Join(path, "node"), "#!/usr/bin/env bash\nexit 0\n", 0o755)
	if err := os.Symlink("/usr/bin/dirname", filepath.Join(path, "dirname")); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/bash", filepath.Join(root, "scripts", "build-ui.sh"))
	command.Env = []string{"PATH=" + path}
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "npm is required to build the embedded GUI") {
		t.Fatalf("missing npm error=%v output=%q", err, output)
	}
}

func TestReleaseInputsContainNoTrackedGeneratedArtifacts(t *testing.T) {
	root := repoRootFromOmniTest(t)
	output := runFixtureGit(t, root, "ls-files")
	for _, path := range strings.Fields(output) {
		if path == "core" || path == "go.mod.orig" ||
			strings.HasPrefix(path, ".tmp-go-audit-cache/") ||
			strings.HasPrefix(path, ".tmp-ledger-cascade.") ||
			strings.HasPrefix(path, ".codex-ledger-fixture.") ||
			strings.HasPrefix(path, ".migration-manifest-") {
			t.Fatalf("generated release input remains tracked: %s", path)
		}
	}
	ignore := readRepoScript(t, root, ".gitignore")
	for _, rule := range []string{
		"/.tmp-go-audit-cache/", ".tmp-ledger-cascade.*/", ".codex-ledger-fixture.*/", ".migration-manifest-*/", "/core", "/go.mod.orig",
	} {
		if !strings.Contains(ignore, rule) {
			t.Fatalf(".gitignore missing generated-artifact rule %q", rule)
		}
	}
}

func TestEveryCoreDeliveryPathBuildsEmbeddedGUI(t *testing.T) {
	root := repoRootFromOmniTest(t)
	checks := map[string]string{
		"install.sh":               `scripts/build-ui.sh`,
		"update.sh":                `scripts/build-ui.sh`,
		"scripts/build-core.sh":    `build-ui.sh`,
		"scripts/build-release.sh": `prepare_ui_dist`,
		"Dockerfile":               `RUN ./scripts/build-ui.sh`,
	}
	for path, fragment := range checks {
		if body := readRepoScript(t, root, path); !strings.Contains(body, fragment) {
			t.Fatalf("%s does not build the embedded GUI via %q", path, fragment)
		}
	}
}

type managedInstallFixture struct {
	source, prefix, home, fakeBin string
}

func newManagedInstallFixture(t *testing.T) managedInstallFixture {
	t.Helper()
	root := repoRootFromOmniTest(t)
	base := t.TempDir()
	remote := filepath.Join(base, "origin.git")
	source := filepath.Join(base, "source")
	prefix := filepath.Join(base, "install")
	home := filepath.Join(base, "home")
	fakeBin := filepath.Join(base, "fake-bin")
	for _, directory := range []string{source, home, fakeBin} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	runFixtureGit(t, base, "init", "--bare", remote)
	runFixtureGit(t, source, "init", "-b", "main")
	runFixtureGit(t, source, "config", "user.email", "install-test@example.invalid")
	runFixtureGit(t, source, "config", "user.name", "Install Test")
	runFixtureGit(t, source, "remote", "add", "origin", remote)

	for _, path := range []string{"install.sh", "update.sh", "scripts/build-ui.sh", "scripts/install-shell-lib.sh", "scripts/managed-checkout-lib.sh", "scripts/update-runtime-lib.sh"} {
		copyFixtureFile(t, filepath.Join(root, path), filepath.Join(source, path), 0o755)
	}
	writeFixtureFile(t, filepath.Join(source, "scripts", "setup-host-deps.sh"), "#!/usr/bin/env bash\nexit 0\n", 0o755)
	writeFixtureFile(t, filepath.Join(source, "agent_aliases.sh"), "#!/usr/bin/env bash\n", 0o755)
	writeFixtureFile(t, filepath.Join(source, "default.env"), "DEFAULT_SETTING=1\n", 0o644)
	writeFixtureFile(t, filepath.Join(source, "README.md"), "first revision\n", 0o644)
	writeFixtureFile(t, filepath.Join(source, "go.mod"), "module example.invalid/install-fixture\n\ngo 1.24\n", 0o644)
	writeFixtureFile(t, filepath.Join(source, "internal/api/web/package.json"), "{}\n", 0o644)
	writeFixtureFile(t, filepath.Join(source, "internal/api/web/package-lock.json"), "{}\n", 0o644)
	writeFixtureFile(t, filepath.Join(source, ".gitignore"), "/bin/\n.env\ninternal/api/web/node_modules/\ninternal/api/web/dist/\n", 0o644)
	runFixtureGit(t, source, "add", ".")
	runFixtureGit(t, source, "commit", "-m", "initial")
	runFixtureGit(t, source, "push", "-u", "origin", "main")

	writeFixtureFile(t, filepath.Join(home, ".bashrc"), "# fixture\n", 0o644)
	writeFixtureFile(t, filepath.Join(fakeBin, "node"), "#!/usr/bin/env bash\nexit 0\n", 0o755)
	writeFixtureFile(t, filepath.Join(fakeBin, "npm"), `#!/usr/bin/env bash
set -euo pipefail
[[ -z "${OMNI_FIXTURE_NPM_FAIL:-}" ]] || { echo "forced npm failure" >&2; exit 61; }
if [[ "$1" == "ci" ]]; then mkdir -p node_modules; exit 0; fi
if [[ "$*" == "run build" ]]; then
  mkdir -p dist/.vite dist/assets
  printf '<html>fixture</html>\n' > dist/index.html
  printf '{}\n' > dist/.vite/manifest.json
  printf 'fixture\n' > dist/assets/app.js
  exit 0
fi
echo "unexpected npm arguments: $*" >&2
exit 62
`, 0o755)
	writeFixtureFile(t, filepath.Join(fakeBin, "go"), `#!/usr/bin/env bash
set -euo pipefail
[[ -z "${OMNI_FIXTURE_GO_FAIL:-}" ]] || { echo "forced go failure" >&2; exit 63; }
output=""
while (($#)); do
  if [[ "$1" == "-o" ]]; then output="$2"; shift 2; continue; fi
  shift
done
[[ -n "$output" ]] || { echo "missing go output" >&2; exit 64; }
mkdir -p "$(dirname "$output")"
if [[ "$output" == */agent-core ]]; then
  printf '#!/usr/bin/env bash\nset -euo pipefail\nif [[ "${1:-}" == "config:validate-file" ]]; then\n  while IFS= read -r line; do [[ "$line" != APP_ENV=* ]] || exit 65; done < "$2"\nfi\nexit 0\n' > "$output"
else
  printf '#!/usr/bin/env bash\nexit 0\n' > "$output"
fi
chmod 0755 "$output"
`, 0o755)
	return managedInstallFixture{source: source, prefix: prefix, home: home, fakeBin: fakeBin}
}

func runManagedScript(t *testing.T, fixture managedInstallFixture, script string, extraEnv []string, arguments ...string) {
	t.Helper()
	output, err := runManagedScriptResult(fixture, script, extraEnv, arguments...)
	if err != nil {
		t.Fatalf("run %s: %v\n%s", script, err, output)
	}
}

func runManagedScriptResult(fixture managedInstallFixture, script string, extraEnv []string, arguments ...string) (string, error) {
	command := exec.Command("bash", append([]string{script}, arguments...)...)
	command.Env = append(os.Environ(), "HOME="+fixture.home, "PATH="+fixture.fakeBin+":"+os.Getenv("PATH"))
	command.Env = append(command.Env, extraEnv...)
	output, err := command.CombinedOutput()
	return string(output), err
}

func assertManagedCheckoutClean(t *testing.T, repository string) {
	t.Helper()
	if deleted := runFixtureGit(t, repository, "ls-files", "--deleted"); deleted != "" {
		t.Fatalf("managed checkout has tracked deletions: %s", deleted)
	}
	if status := runFixtureGit(t, repository, "status", "--porcelain=1", "--untracked-files=all"); status != "" {
		t.Fatalf("managed checkout is dirty after build: %s", status)
	}
}

func runFixtureGit(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git -C %s %v: %v: %s", directory, arguments, err, output)
	}
	return strings.TrimSpace(string(output))
}

func writeFixtureFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func copyFixtureFile(t *testing.T, source, target string, mode os.FileMode) {
	t.Helper()
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, target, string(raw), mode)
}

func readFixtureFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
