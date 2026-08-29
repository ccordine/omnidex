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
	environment := managedCheckoutTestEnvironment(t)
	runManagedScript(t, fixture, filepath.Join(fixture.source, "install.sh"), nil,
		"--prefix", fixture.prefix, "--env-file", environment, "--skip-deps", "--yes")

	assertManagedCheckoutClean(t, fixture.prefix)
	oldHead := runFixtureGit(t, fixture.prefix, "rev-parse", "HEAD")
	oldBinary := readFixtureFile(t, filepath.Join(fixture.prefix, "bin", "omni"))
	preservedEnvironment := "CORE_URL=https://managed.example\nUSER_SETTING=preserved\n"
	writeFixtureFile(t, filepath.Join(fixture.prefix, ".env"), preservedEnvironment, 0o600)

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
		if got := readFixtureFile(t, filepath.Join(fixture.prefix, ".env")); got != preservedEnvironment {
			t.Fatalf("failed update changed .env: %q", got)
		}
	}

	runManagedScript(t, fixture, filepath.Join(fixture.prefix, "update.sh"), nil,
		"--host-only", "--no-host-restart")
	if got := runFixtureGit(t, fixture.prefix, "rev-parse", "HEAD"); got != newHead {
		t.Fatalf("updated HEAD=%s want=%s", got, newHead)
	}
	if got := readFixtureFile(t, filepath.Join(fixture.prefix, ".env")); got != preservedEnvironment {
		t.Fatalf("successful update did not preserve .env: %q", got)
	}
	assertManagedCheckoutClean(t, fixture.prefix)
}

func TestManagedUpdateRejectsIncompatiblePreservedEnvironmentBeforePublish(t *testing.T) {
	fixture := newManagedInstallFixture(t)
	environment := managedCheckoutTestEnvironment(t)
	runManagedScript(t, fixture, filepath.Join(fixture.source, "install.sh"), nil,
		"--prefix", fixture.prefix, "--env-file", environment, "--skip-deps", "--yes")
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

func TestManagedCheckoutFreshInstallRequiresExplicitExactEnvironment(t *testing.T) {
	fixture := newManagedInstallFixture(t)
	output, err := runManagedScriptResult(fixture, filepath.Join(fixture.source, "install.sh"), nil,
		"--prefix", fixture.prefix, "--skip-deps", "--yes")
	if err == nil || !strings.Contains(output, "requires --env-file PATH") {
		t.Fatalf("missing fresh environment error=%v output=%s", err, output)
	}
	if _, statErr := os.Stat(fixture.prefix); !os.IsNotExist(statErr) {
		t.Fatalf("missing environment published checkout: %v", statErr)
	}

	exact := "CORE_URL=https://managed.example\nSECRET_VALUE=exact bytes  \n"
	environment := filepath.Join(t.TempDir(), "deployment.env")
	writeFixtureFile(t, environment, exact, 0o600)
	runManagedScript(t, fixture, filepath.Join(fixture.source, "install.sh"), nil,
		"--prefix", fixture.prefix, "--env-file", environment, "--skip-deps", "--yes")
	if got := readFixtureFile(t, filepath.Join(fixture.prefix, ".env")); got != exact {
		t.Fatalf("managed checkout environment=%q want exact %q", got, exact)
	}
}

func TestManagedCheckoutFreshInstallRejectsInvalidAndNonRegularEnvironment(t *testing.T) {
	for _, test := range []struct {
		name string
		make func(*testing.T) string
		want string
	}{
		{name: "invalid", make: func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "invalid.env")
			writeFixtureFile(t, path, "APP_ENV=production\n", 0o600)
			return path
		}, want: "staged .env is incompatible"},
		{name: "missing CORE_URL", make: func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "missing-core-url.env")
			writeFixtureFile(t, path, "USER_SETTING=value\n", 0o600)
			return path
		}, want: "does not provide valid managed CLI authority"},
		{name: "directory", make: func(t *testing.T) string { return t.TempDir() }, want: "regular non-symlink"},
		{name: "symlink", make: func(t *testing.T) string {
			directory := t.TempDir()
			real := filepath.Join(directory, "real.env")
			link := filepath.Join(directory, "link.env")
			writeFixtureFile(t, real, "CORE_URL=https://managed.example\n", 0o600)
			if err := os.Symlink(real, link); err != nil {
				t.Fatal(err)
			}
			return link
		}, want: "regular non-symlink"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newManagedInstallFixture(t)
			output, err := runManagedScriptResult(fixture, filepath.Join(fixture.source, "install.sh"), nil,
				"--prefix", fixture.prefix, "--env-file", test.make(t), "--skip-deps", "--yes")
			if err == nil || !strings.Contains(output, test.want) {
				t.Fatalf("environment rejection error=%v output=%s", err, output)
			}
			if _, statErr := os.Stat(fixture.prefix); !os.IsNotExist(statErr) {
				t.Fatalf("failed install published checkout: %v", statErr)
			}
		})
	}
}

func TestManagedCheckoutInstallRejectsExplicitEnvironmentReplacement(t *testing.T) {
	fixture := newManagedInstallFixture(t)
	environment := managedCheckoutTestEnvironment(t)
	runManagedScript(t, fixture, filepath.Join(fixture.source, "install.sh"), nil,
		"--prefix", fixture.prefix, "--env-file", environment, "--skip-deps", "--yes")
	replacement := filepath.Join(t.TempDir(), "replacement.env")
	writeFixtureFile(t, replacement, "CORE_URL=https://replacement.example\n", 0o600)
	output, err := runManagedScriptResult(fixture, filepath.Join(fixture.source, "install.sh"), nil,
		"--prefix", fixture.prefix, "--env-file", replacement, "--skip-deps", "--yes")
	if err == nil || !strings.Contains(output, "cannot replace an existing managed .env") {
		t.Fatalf("managed replacement error=%v output=%s", err, output)
	}
	if got := readFixtureFile(t, filepath.Join(fixture.prefix, ".env")); got != "USER_SETTING=preserved\n" && got != readFixtureFile(t, environment) {
		t.Fatalf("rejected replacement changed environment: %q", got)
	}
}

func TestManagedUpdateRejectsNonFastForwardWithoutChangingActiveInstall(t *testing.T) {
	fixture := newManagedInstallFixture(t)
	environment := managedCheckoutTestEnvironment(t)
	runManagedScript(t, fixture, filepath.Join(fixture.source, "install.sh"), nil,
		"--prefix", fixture.prefix, "--env-file", environment, "--skip-deps", "--yes")
	oldHead := runFixtureGit(t, fixture.prefix, "rev-parse", "HEAD")
	oldBinary := readFixtureFile(t, filepath.Join(fixture.prefix, "bin", "omni"))
	oldEnvironment := readFixtureFile(t, filepath.Join(fixture.prefix, ".env"))

	writeFixtureFile(t, filepath.Join(fixture.source, "README.md"), "remote revision\n", 0o644)
	runFixtureGit(t, fixture.source, "add", "README.md")
	runFixtureGit(t, fixture.source, "commit", "-m", "remote revision")
	runFixtureGit(t, fixture.source, "push", "origin", "main")
	writeFixtureFile(t, filepath.Join(fixture.prefix, "README.md"), "local divergent revision\n", 0o644)
	runFixtureGit(t, fixture.prefix, "add", "README.md")
	runFixtureGit(t, fixture.prefix, "config", "user.email", "install-test@example.invalid")
	runFixtureGit(t, fixture.prefix, "config", "user.name", "Install Test")
	runFixtureGit(t, fixture.prefix, "commit", "-m", "local divergence")
	divergentHead := runFixtureGit(t, fixture.prefix, "rev-parse", "HEAD")

	output, err := runManagedScriptResult(fixture, filepath.Join(fixture.prefix, "update.sh"), nil,
		"--host-only", "--no-host-restart")
	if err == nil || !strings.Contains(output, "Not possible to fast-forward") {
		t.Fatalf("non-fast-forward error=%v output=%s", err, output)
	}
	if got := runFixtureGit(t, fixture.prefix, "rev-parse", "HEAD"); got != divergentHead || got == oldHead {
		t.Fatalf("failed non-FF update changed active HEAD: got=%s divergent=%s old=%s", got, divergentHead, oldHead)
	}
	if got := readFixtureFile(t, filepath.Join(fixture.prefix, "bin", "omni")); got != oldBinary {
		t.Fatal("failed non-FF update changed active omni binary")
	}
	if got := readFixtureFile(t, filepath.Join(fixture.prefix, ".env")); got != oldEnvironment {
		t.Fatalf("failed non-FF update changed active environment: %q", got)
	}
}

func TestManagedCheckoutPublicationFailureRollsBackPriorInstall(t *testing.T) {
	root := repoRootFromOmniTest(t)
	parent := t.TempDir()
	target := filepath.Join(parent, "install")
	stage := filepath.Join(parent, ".install.update.fixture")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, filepath.Join(target, "marker"), "prior install\n", 0o600)
	writeFixtureFile(t, filepath.Join(stage, "marker"), "new install\n", 0o600)
	runFixtureGit(t, target, "init", "-b", "main")
	runFixtureGit(t, target, "config", "user.email", "install-test@example.invalid")
	runFixtureGit(t, target, "config", "user.name", "Install Test")
	runFixtureGit(t, target, "add", "marker")
	runFixtureGit(t, target, "commit", "-m", "prior install")

	realMove, err := exec.LookPath("mv")
	if err != nil {
		t.Fatal(err)
	}
	fakeBin := t.TempDir()
	fakeMove := `#!/usr/bin/env bash
case "${1:-}" in
  *.install.update.fixture) printf 'forced checkout publication failure\n' >&2; exit 71 ;;
  *) exec ` + shellQuote(realMove) + ` "$@" ;;
esac
`
	writeFixtureFile(t, filepath.Join(fakeBin, "mv"), fakeMove, 0o700)
	script := `
set -euo pipefail
source "$1/scripts/managed-checkout-lib.sh"
die() { printf '%s\n' "$*" >&2; exit 1; }
managed_checkout_publish "$2" "$3"
`
	command := exec.Command("bash", "-c", script, "checkout-publish-rollback", root, stage, target)
	command.Env = append(os.Environ(), "PATH="+fakeBin+":"+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "failed to publish staged checkout") {
		t.Fatalf("checkout publication error=%v output=%s", err, output)
	}
	if got := readFixtureFile(t, filepath.Join(target, "marker")); got != "prior install\n" {
		t.Fatalf("checkout rollback lost prior install: %q", got)
	}
	if got := readFixtureFile(t, filepath.Join(stage, "marker")); got != "new install\n" {
		t.Fatalf("failed checkout publication changed stage: %q", got)
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
			strings.HasPrefix(path, ".codex-ledger-fixture.") {
			t.Fatalf("generated release input remains tracked: %s", path)
		}
	}
	ignore := readRepoScript(t, root, ".gitignore")
	for _, rule := range []string{
		"/.tmp-go-audit-cache/", ".tmp-ledger-cascade.*/", ".codex-ledger-fixture.*/", "/core", "/go.mod.orig",
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

func TestDockerBuilderInstallsBuildScriptInterpreter(t *testing.T) {
	root := repoRootFromOmniTest(t)
	dockerfile := readRepoScript(t, root, "Dockerfile")
	if !strings.Contains(dockerfile, "apk add --no-cache bash build-base nodejs npm") {
		t.Fatal("Docker builder must install bash before running scripts/build-ui.sh")
	}
}

func TestDockerRuntimeCopiesOnlyExistingAuthoritativeInputs(t *testing.T) {
	root := repoRootFromOmniTest(t)
	dockerfile := readRepoScript(t, root, "Dockerfile")
	if !strings.Contains(dockerfile, "COPY --from=build /src/database/setup.sql /usr/local/database/setup.sql") {
		t.Fatal("Docker runtime must retain the authoritative database setup")
	}
	if strings.Contains(dockerfile, "COPY --from=build /src/migrations") {
		t.Fatal("Docker runtime retains a numbered database history")
	}
	if info, err := os.Stat(filepath.Join(root, "database", "setup.sql")); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("Docker runtime setup must be one existing regular file: %v", err)
	}
}

func TestDockerReleaseIdentityContractIsExplicitAndContextBounded(t *testing.T) {
	root := repoRootFromOmniTest(t)
	dockerfile := readRepoScript(t, root, "Dockerfile")
	for _, fragment := range []string{
		"ARG OMNIDEX_COMMIT",
		"ARG APP_UID=1000",
		"ARG APP_GID=1000",
		`validate_host_id APP_UID "${APP_UID}"`,
		`validate_host_id APP_GID "${APP_GID}"`,
		`USER ${APP_UID}:${APP_GID}`,
		`${#OMNIDEX_COMMIT}`,
		`*[!0123456789abcdef]*`,
		"go build -trimpath",
		"internal/version.Commit=${OMNIDEX_COMMIT}",
		`LABEL org.opencontainers.image.revision="${OMNIDEX_COMMIT}"`,
	} {
		if !strings.Contains(dockerfile, fragment) {
			t.Fatalf("Dockerfile omits release identity contract %q", fragment)
		}
	}
	compose := readRepoScript(t, root, "docker-compose.yml")
	if !strings.Contains(compose, `OMNIDEX_COMMIT: ${OMNIDEX_COMMIT:-}`) {
		t.Fatal("Compose build does not accept an explicit commit with a blank non-build default")
	}
	for _, fragment := range []string{
		`APP_UID: ${HOST_UID:?HOST_UID must match the owner of HOST_WORKSPACE_PATH}`,
		`APP_GID: ${HOST_GID:?HOST_GID must match the group of HOST_WORKSPACE_PATH}`,
		`group_add:`,
		`${DOCKER_GID:?DOCKER_GID must match the numeric group owner of /var/run/docker.sock}`,
	} {
		if !strings.Contains(compose, fragment) {
			t.Fatalf("Compose omits exact host runtime identity %q", fragment)
		}
	}
	ignore := readRepoScript(t, root, ".dockerignore")
	for _, entry := range []string{".git", "dist"} {
		if !strings.Contains("\n"+ignore, "\n"+entry+"\n") {
			t.Fatalf("Docker build context does not exclude %q", entry)
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
	writeFixtureFile(t, filepath.Join(source, ".gitignore"), "/bin/\n.env\ninternal/api/web/node_modules/\ninternal/api/web/dist/\nlogs/\n.omni/\n", 0o644)
	runFixtureGit(t, source, "add", ".")
	runFixtureGit(t, source, "commit", "-m", "initial")
	runFixtureGit(t, source, "push", "-u", "origin", "main")

	writeFixtureFile(t, filepath.Join(home, ".bashrc"), "# fixture\n", 0o644)
	writeFixtureFile(t, filepath.Join(fakeBin, "node"), "#!/usr/bin/env bash\nexit 0\n", 0o755)
	writeFixtureFile(t, filepath.Join(fakeBin, "npm"), `#!/usr/bin/env bash
set -euo pipefail
[[ -z "${OMNI_FIXTURE_NPM_FAIL:-}" ]] || { echo "forced npm failure" >&2; exit 61; }
if [[ "$1" == "ci" ]]; then
  [[ " $* " == *" --include=dev "* ]] || { echo "npm ci must include development build dependencies" >&2; exit 67; }
  mkdir -p node_modules/esbuild/bin
  printf '#!/usr/bin/env bash\nexit 0\n' > node_modules/esbuild/bin/esbuild
  chmod 0755 node_modules/esbuild/bin/esbuild
  exit 0
fi
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
if [[ "${1:-}" == "version" && "${2:-}" == "-m" ]]; then
  printf '%s: go1.24\n\tpath\tfixture\n\tbuild\t-trimpath=true\n\tbuild\tvcs.revision=%s\n\tbuild\tvcs.modified=false\n' "$3" "${OMNIDEX_COMMIT}"
  exit 0
fi
output=""
while (($#)); do
  if [[ "$1" == "-o" ]]; then output="$2"; shift 2; continue; fi
  shift
done
[[ -n "$output" ]] || { echo "missing go output" >&2; exit 64; }
mkdir -p "$(dirname "$output")"
if [[ "$output" == */agent-core ]]; then
  printf '#!/usr/bin/env bash\nset -euo pipefail\nif [[ "${1:-}" == "release:verify-commit" ]]; then [[ "${2:-}" == "${OMNIDEX_COMMIT}" ]] || exit 68; printf "%%s\\n" "${OMNIDEX_COMMIT}"; exit 0; fi\nif [[ "${1:-}" == "config:validate-file" ]]; then\n  while IFS= read -r line; do [[ "$line" != APP_ENV=* ]] || exit 65; done < "$2"\nfi\nexit 0\n' > "$output"
elif [[ "$output" == */agent-cli ]]; then
  printf '#!/usr/bin/env bash\nset -euo pipefail\nif [[ "${1:-}" == "version" && "${2:-}" == "--json" ]]; then printf "{\\n  \\"commit\\": \\"%%s\\"\\n}\\n" "${OMNIDEX_COMMIT}"; exit 0; fi\nif [[ "${1:-}" == "config:validate-file" ]]; then\n  count=0; value=""\n  while IFS= read -r line; do case "$line" in CORE_URL=*) count=$((count + 1)); value="${line#CORE_URL=}" ;; esac; done < "$2"\n  [[ $count -eq 1 && ( "$value" == http://* || "$value" == https://* ) ]] || exit 66\nfi\nexit 0\n' > "$output"
else
  printf '#!/usr/bin/env bash\nset -euo pipefail\nif [[ "${1:-}" == "version" && "${2:-}" == "--json" ]]; then printf "{\\n  \\"commit\\": \\"%%s\\"\\n}\\n" "${OMNIDEX_COMMIT}"; exit 0; fi\nexit 0\n' > "$output"
fi
chmod 0755 "$output"
`, 0o755)
	return managedInstallFixture{source: source, prefix: prefix, home: home, fakeBin: fakeBin}
}

func managedCheckoutTestEnvironment(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "deployment.env")
	writeFixtureFile(t, path, "CORE_URL=https://managed.example\n", 0o600)
	return path
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
