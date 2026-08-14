package omni

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedReleaseFreshInstallRequiresAndPreservesExplicitEnvironment(t *testing.T) {
	fixture := newManagedReleaseFixture(t)
	environment := filepath.Join(t.TempDir(), "deployment.env")
	exact := "CORE_URL=https://managed.example\nSECRET_VALUE=exact bytes  \n"
	writeFixtureFile(t, environment, exact, 0o600)

	output, err := runManagedReleaseInstaller(fixture, nil,
		"--prefix", fixture.target, "--env-file", environment, "--yes")
	if err != nil {
		t.Fatalf("fresh managed release install: %v: %s", err, output)
	}
	if got := readFixtureFile(t, filepath.Join(fixture.target, ".env")); got != exact {
		t.Fatalf("installed environment = %q, want exact %q", got, exact)
	}
	if got := readFixtureFile(t, filepath.Join(fixture.target, "default.env")); got != "TEMPLATE_ONLY=1\n" {
		t.Fatalf("release template was changed or promoted: %q", got)
	}
	for _, binary := range []string{"omni", "agent-cli", "agent-core"} {
		if info, err := os.Stat(filepath.Join(fixture.target, "bin", binary)); err != nil || info.Mode()&0o111 == 0 {
			t.Fatalf("installed %s is not executable: %v", binary, err)
		}
	}
	if shell := readFixtureFile(t, filepath.Join(fixture.home, ".bashrc")); !strings.Contains(shell, fixture.target) {
		t.Fatalf("shell integration does not reference installed release: %q", shell)
	}
}

func TestManagedReleaseFreshInstallRejectsMissingMalformedAndUnsafeEnvironment(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*testing.T) string
		want      string
	}{
		{name: "missing flag", configure: func(*testing.T) string { return "" }, want: "requires --env-file PATH"},
		{name: "invalid config", configure: func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "invalid.env")
			writeFixtureFile(t, path, "INVALID=1\n", 0o600)
			return path
		}, want: "staged .env is incompatible"},
		{name: "missing CORE_URL", configure: func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "missing-core-url.env")
			writeFixtureFile(t, path, "OTHER=value\n", 0o600)
			return path
		}, want: "does not provide valid managed CLI authority"},
		{name: "directory", configure: func(t *testing.T) string { return t.TempDir() }, want: "regular non-symlink"},
		{name: "symlink", configure: func(t *testing.T) string {
			directory := t.TempDir()
			target := filepath.Join(directory, "real.env")
			link := filepath.Join(directory, "link.env")
			writeFixtureFile(t, target, "CORE_URL=https://managed.example\n", 0o600)
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			return link
		}, want: "regular non-symlink"},
		{name: "inside release", configure: func(t *testing.T) string {
			return "inside-release"
		}, want: "outside the extracted release directory"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newManagedReleaseFixture(t)
			environment := test.configure(t)
			if environment == "inside-release" {
				environment = filepath.Join(fixture.release, "deployment.env")
				writeFixtureFile(t, environment, "CORE_URL=https://managed.example\n", 0o600)
			}
			arguments := []string{"--prefix", fixture.target, "--yes"}
			if environment != "" {
				arguments = append(arguments, "--env-file", environment)
			}
			output, err := runManagedReleaseInstaller(fixture, nil, arguments...)
			if err == nil || !strings.Contains(output, test.want) {
				t.Fatalf("environment rejection error = %v, output = %q", err, output)
			}
			if _, statErr := os.Stat(fixture.target); !os.IsNotExist(statErr) {
				t.Fatalf("failed fresh install published target: %v", statErr)
			}
			stages, globErr := filepath.Glob(filepath.Join(filepath.Dir(fixture.target), ".install.release-install.*"))
			if globErr != nil || len(stages) != 0 {
				t.Fatalf("failed fresh install left stages %v (error %v)", stages, globErr)
			}
		})
	}
}

func TestManagedReleaseUpgradePreservesExistingEnvironmentAndRejectsReplacement(t *testing.T) {
	fixture := newManagedReleaseFixture(t)
	environment := filepath.Join(t.TempDir(), "deployment.env")
	exact := "CORE_URL=https://managed.example\nSECRET_VALUE=keep-this-exactly\n"
	writeFixtureFile(t, environment, exact, 0o600)
	if output, err := runManagedReleaseInstaller(fixture, nil,
		"--prefix", fixture.target, "--env-file", environment, "--yes"); err != nil {
		t.Fatalf("initial install: %v: %s", err, output)
	}
	writeFixtureFile(t, filepath.Join(fixture.release, "release-marker"), "second release\n", 0o644)
	if output, err := runManagedReleaseInstaller(fixture, nil, "--prefix", fixture.target, "--yes"); err != nil {
		t.Fatalf("release upgrade: %v: %s", err, output)
	}
	if got := readFixtureFile(t, filepath.Join(fixture.target, ".env")); got != exact {
		t.Fatalf("upgrade changed managed environment: %q", got)
	}
	if got := readFixtureFile(t, filepath.Join(fixture.target, "release-marker")); got != "second release\n" {
		t.Fatalf("upgrade did not publish new release: %q", got)
	}

	replacement := filepath.Join(t.TempDir(), "replacement.env")
	writeFixtureFile(t, replacement, "CORE_URL=https://replacement.example\n", 0o600)
	output, err := runManagedReleaseInstaller(fixture, nil,
		"--prefix", fixture.target, "--env-file", replacement, "--yes")
	if err == nil || !strings.Contains(output, "cannot replace an existing managed .env") {
		t.Fatalf("managed environment replacement error = %v, output = %q", err, output)
	}
	if got := readFixtureFile(t, filepath.Join(fixture.target, ".env")); got != exact {
		t.Fatalf("rejected replacement changed environment: %q", got)
	}
}

func TestManagedReleaseUpgradeRejectsNonRegularExistingEnvironment(t *testing.T) {
	fixture := newManagedReleaseFixture(t)
	environment := filepath.Join(t.TempDir(), "deployment.env")
	writeFixtureFile(t, environment, "CORE_URL=https://managed.example\n", 0o600)
	if output, err := runManagedReleaseInstaller(fixture, nil,
		"--prefix", fixture.target, "--env-file", environment, "--yes"); err != nil {
		t.Fatalf("initial install: %v: %s", err, output)
	}
	if err := os.Remove(filepath.Join(fixture.target, ".env")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(environment, filepath.Join(fixture.target, ".env")); err != nil {
		t.Fatal(err)
	}
	output, err := runManagedReleaseInstaller(fixture, nil, "--prefix", fixture.target, "--yes")
	if err == nil || !strings.Contains(output, "existing managed .env must be a regular file") {
		t.Fatalf("existing symlink environment error = %v, output = %q", err, output)
	}
	if link, linkErr := os.Readlink(filepath.Join(fixture.target, ".env")); linkErr != nil || link != environment {
		t.Fatalf("rejected upgrade changed existing symlink: link=%q error=%v", link, linkErr)
	}
}

func TestManagedReleasePublicationFailureRollsBackPriorInstall(t *testing.T) {
	fixture := newManagedReleaseFixture(t)
	environment := filepath.Join(t.TempDir(), "deployment.env")
	exact := "CORE_URL=https://managed.example\nSECRET_VALUE=rollback\n"
	writeFixtureFile(t, environment, exact, 0o600)
	if output, err := runManagedReleaseInstaller(fixture, nil,
		"--prefix", fixture.target, "--env-file", environment, "--yes"); err != nil {
		t.Fatalf("initial install: %v: %s", err, output)
	}
	writeFixtureFile(t, filepath.Join(fixture.target, "active-marker"), "prior install\n", 0o600)
	writeFixtureFile(t, filepath.Join(fixture.release, "release-marker"), "unpublished release\n", 0o644)

	realMove, err := exec.LookPath("mv")
	if err != nil {
		t.Fatal(err)
	}
	fakeBin := t.TempDir()
	fakeMove := `#!/usr/bin/env bash
case "${1:-}" in
  *.release-install.*) printf 'forced publication failure\n' >&2; exit 71 ;;
  *) exec ` + shellQuote(realMove) + ` "$@" ;;
esac
`
	writeFixtureFile(t, filepath.Join(fakeBin, "mv"), fakeMove, 0o700)
	output, err := runManagedReleaseInstaller(fixture, []string{"PATH=" + fakeBin + ":" + os.Getenv("PATH")},
		"--prefix", fixture.target, "--yes")
	if err == nil || !strings.Contains(output, "failed to publish staged release") {
		t.Fatalf("forced publication error = %v, output = %q", err, output)
	}
	if got := readFixtureFile(t, filepath.Join(fixture.target, "active-marker")); got != "prior install\n" {
		t.Fatalf("rollback lost prior install: %q", got)
	}
	if got := readFixtureFile(t, filepath.Join(fixture.target, ".env")); got != exact {
		t.Fatalf("rollback changed prior environment: %q", got)
	}
	if _, statErr := os.Stat(filepath.Join(fixture.target, "release-marker")); !os.IsNotExist(statErr) {
		t.Fatalf("failed publication exposed new release: %v", statErr)
	}
}

type managedReleaseFixture struct {
	release string
	target  string
	home    string
}

func newManagedReleaseFixture(t *testing.T) managedReleaseFixture {
	t.Helper()
	root := repoRootFromOmniTest(t)
	base := t.TempDir()
	fixture := managedReleaseFixture{
		release: filepath.Join(base, "release"),
		target:  filepath.Join(base, "install"),
		home:    filepath.Join(base, "home"),
	}
	for _, directory := range []string{filepath.Join(fixture.release, "bin"), filepath.Join(fixture.release, "scripts"), fixture.home} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"install-release.sh", "scripts/managed-release-install-lib.sh", "scripts/install-shell-lib.sh"} {
		copyFixtureFile(t, filepath.Join(root, path), filepath.Join(fixture.release, path), 0o700)
	}
	writeFixtureFile(t, filepath.Join(fixture.release, "default.env"), "TEMPLATE_ONLY=1\n", 0o600)
	writeFixtureFile(t, filepath.Join(fixture.release, "bin", "omni"), "#!/usr/bin/env bash\nexit 0\n", 0o700)
	writeFixtureFile(t, filepath.Join(fixture.release, "bin", "agent-cli"), `#!/usr/bin/env bash
set -euo pipefail
[[ "${1:-}" == "config:validate-file" && $# -eq 2 ]] || exit 64
count=0
value=""
while IFS= read -r line; do
  case "$line" in
    CORE_URL=*) count=$((count + 1)); value="${line#CORE_URL=}" ;;
  esac
done < "$2"
[[ $count -eq 1 && ( "$value" == http://* || "$value" == https://* ) ]] || exit 66
`, 0o700)
	writeFixtureFile(t, filepath.Join(fixture.release, "bin", "agent-core"), `#!/usr/bin/env bash
set -euo pipefail
[[ "${1:-}" == "config:validate-file" && $# -eq 2 ]] || exit 64
while IFS= read -r line; do
  [[ "$line" != INVALID=* ]] || exit 65
done < "$2"
`, 0o700)
	writeFixtureFile(t, filepath.Join(fixture.home, ".bashrc"), "# test shell\n", 0o600)
	return fixture
}

func runManagedReleaseInstaller(fixture managedReleaseFixture, extraEnvironment []string, arguments ...string) (string, error) {
	command := exec.Command("bash", append([]string{filepath.Join(fixture.release, "install-release.sh")}, arguments...)...)
	command.Env = append(os.Environ(), "HOME="+fixture.home)
	command.Env = append(command.Env, extraEnvironment...)
	output, err := command.CombinedOutput()
	return string(output), err
}
