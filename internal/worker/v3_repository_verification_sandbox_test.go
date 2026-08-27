package worker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func TestRepositoryGoVerificationReportsExactGoCommandNotBubblewrap(t *testing.T) {
	t.Parallel()
	_, projection, config, view := repositorySandboxFixture(t, fakeBubblewrapScript(0, "", 0))
	result, err := executeRepositoryGoVerificationWithConfig(
		context.Background(), projection, repositoryGoTestCall(0), config, view,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !directCodingCommandSucceeded(result) {
		t.Fatalf("sandbox command result=%#v", result.Output)
	}
	if got := operationResultText(result.Output, "stdout"); !strings.Contains(got, "ok\texample.com/sandbox") {
		t.Fatalf("stdout=%q", got)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Command != "go test -json -count=1 ./..." {
		t.Fatalf("evidence=%#v", result.Evidence)
	}
	encoded := result.Evidence[0].Command + result.Evidence[0].Summary + result.Evidence[0].Excerpt
	if strings.Contains(encoded, "bwrap") || strings.Contains(encoded, config.BubblewrapPath) {
		t.Fatalf("bubblewrap internals escaped exact command evidence: %q", encoded)
	}
}

func TestRepositoryGoVerificationPreservesFailedGoTestAsEvidence(t *testing.T) {
	t.Parallel()
	_, projection, config, view := repositorySandboxFixture(t, fakeBubblewrapScript(1, "", 0))
	result, err := executeRepositoryGoVerificationWithConfig(
		context.Background(), projection, repositoryGoTestCall(0), config, view,
	)
	if err != nil {
		t.Fatal(err)
	}
	if directCodingCommandSucceeded(result) || result.Output["exit_code"] != 1 {
		t.Fatalf("failed command result=%#v", result.Output)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Command != "go test -json -count=1 ./..." ||
		len(result.Evidence[0].Warnings) != 1 {
		t.Fatalf("failed evidence=%#v", result.Evidence)
	}
}

func TestRepositoryGoVerificationRejectsUnregisteredAndSignaledExitSemantics(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, script string
	}{
		{name: "unregistered", script: fakeBubblewrapScript(7, "", 0)},
		{name: "signaled", script: fakeBubblewrapScript(0, "kill -TERM $$", 0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, projection, config, view := repositorySandboxFixture(t, test.script)
			result, err := executeRepositoryGoVerificationWithConfig(
				context.Background(), projection, repositoryGoTestCall(0), config, view,
			)
			if err == nil || !strings.Contains(err.Error(), "unregistered exit semantics") {
				t.Fatalf("exit result=%#v error=%v", result.Output, err)
			}
			if len(result.Evidence) != 1 {
				t.Fatalf("fatal exit omitted exact evidence: %#v", result.Evidence)
			}
		})
	}
}

func TestRepositoryGoVerificationFailsWithoutSandboxAndNeverRunsCommand(t *testing.T) {
	t.Parallel()
	root, projection, config, view := repositorySandboxFixture(t, fakeBubblewrapScript(0, "", 0))
	config.BubblewrapPath = filepath.Join(t.TempDir(), "missing-bwrap")
	if _, err := executeRepositoryGoVerificationWithConfig(
		context.Background(), projection, repositoryGoTestCall(0), config, view,
	); err == nil || !strings.Contains(err.Error(), "bubblewrap") {
		t.Fatalf("missing sandbox error=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "sandbox-ran")); !os.IsNotExist(err) {
		t.Fatalf("unsandboxed fallback executed: %v", err)
	}
}

func TestRepositoryGoVerificationHonorsTimeoutAndContext(t *testing.T) {
	t.Parallel()
	_, projection, config, view := repositorySandboxFixture(t, fakeBubblewrapScript(0, "", 5))
	started := time.Now()
	if _, err := executeRepositoryGoVerificationWithConfig(
		context.Background(), projection, repositoryGoTestCall(1), config, view,
	); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("timeout error=%v", err)
	}
	if elapsed := time.Since(started); elapsed > 4*time.Second {
		t.Fatalf("sandbox timeout took %s", elapsed)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := executeRepositoryGoVerificationWithConfig(
		canceled, projection, repositoryGoTestCall(0), config, view,
	); err == nil || !strings.Contains(err.Error(), "authority ended") {
		t.Fatalf("canceled context error=%v", err)
	}
}

func TestRepositoryGoVerificationRejectsFilesystemDriftAfterCommand(t *testing.T) {
	t.Parallel()
	script := fakeBubblewrapScript(0, "printf 'drift\\n' > /proc/self/fd/3/unexpected.txt", 0)
	_, projection, config, view := repositorySandboxFixture(t, script)
	result, err := executeRepositoryGoVerificationWithConfig(
		context.Background(), projection, repositoryGoTestCall(0), config, view,
	)
	if err == nil || !strings.Contains(err.Error(), "source state changed") {
		t.Fatalf("filesystem drift error=%v", err)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Command != "go test -json -count=1 ./..." {
		t.Fatalf("drift discarded exact command evidence=%#v", result.Evidence)
	}
}

func TestRepositoryGoVerificationRejectsAnyNonGoTestCommand(t *testing.T) {
	t.Parallel()
	_, projection, config, view := repositorySandboxFixture(t, fakeBubblewrapScript(0, "", 0))
	request := repositoryGoVerificationRequest{Args: []string{"build", "./..."}}
	if _, err := executeRepositoryGoVerificationWithConfig(
		context.Background(), projection, request, config, view,
	); err == nil || !strings.Contains(err.Error(), "only go test") {
		t.Fatalf("non-test command error=%v", err)
	}
}

func TestRepositoryGoVerificationRequestRejectsNonGoExecutable(t *testing.T) {
	t.Parallel()
	_, err := repositoryGoVerificationRequestFromCommand(testCommand{
		Name: "npm", Args: []string{"test", "-json", "-count=1", "./..."},
	})
	if err == nil || !strings.Contains(err.Error(), "exact go executable") {
		t.Fatalf("non-Go executable error=%v", err)
	}
}

func TestRepositoryGoVerificationMountsSourceReadOnly(t *testing.T) {
	t.Parallel()
	_, projection, _, _ := repositorySandboxFixture(t, fakeBubblewrapScript(0, "", 0))
	arguments, err := repositoryGoSandboxArguments(
		projection, repositoryWorkspaceProjectionMountRoots{base: projection.source.Root},
		3, -1, 4, 5, 6,
	)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(arguments, " ")
	if !strings.Contains(joined, "--ro-bind /proc/self/fd/3/go.mod /workspace/go.mod") ||
		!strings.Contains(joined, "--remount-ro "+repositorySandboxRoot) {
		t.Fatalf("repository projection is not read-only: %q", joined)
	}
	if strings.Contains(joined, "--bind-fd 3 "+repositorySandboxRoot) ||
		strings.Contains(joined, "--ro-bind-fd 3 "+repositorySandboxRoot) {
		t.Fatalf("repository source retains a writable bind: %q", joined)
	}
}

func TestRepositoryGoVerificationRejectsUnstructuredGoTestCommand(t *testing.T) {
	t.Parallel()
	_, projection, config, view := repositorySandboxFixture(t, fakeBubblewrapScript(0, "", 0))
	request := repositoryGoVerificationRequest{Args: []string{"test", "./..."}}
	if _, err := executeRepositoryGoVerificationWithConfig(
		context.Background(), projection, request, config, view,
	); err == nil || !strings.Contains(err.Error(), "structured") {
		t.Fatalf("unstructured test command error=%v", err)
	}
}

func TestRepositoryGoVerificationAcceptsOnlyRegisteredFocusedCommandShape(t *testing.T) {
	t.Parallel()
	valid := repositoryGoVerificationRequest{
		Args: []string{"test", "-json", "-count=1", "-run", "^TestOne$", "./sample"},
	}
	args, _, err := prepareRepositoryGoVerificationRequest(valid)
	if err != nil {
		t.Fatal(err)
	}
	if !slicesEqualStrings(args, valid.Args) {
		t.Fatalf("focused args=%v", args)
	}
	for _, selector := range []string{"TestOne", "^($"} {
		invalid := valid
		invalid.Args = []string{"test", "-json", "-count=1", "-run", selector, "./sample"}
		if _, _, err := prepareRepositoryGoVerificationRequest(invalid); err == nil ||
			!strings.Contains(err.Error(), "structured") {
			t.Fatalf("invalid selector %q error=%v", selector, err)
		}
	}
}

func TestRepositoryChangeVerificationHasNoUnsandboxedFallback(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("v3_repository_change_commands.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.Contains(source, "executeRepositoryGoVerification(") {
		t.Fatal("repository verification does not call its authoritative sandbox")
	}
	for _, forbidden := range []string{
		"executeV3CommandAtRoot(", "exec.Command(", "exec.CommandContext(",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("repository verification retains unsandboxed fallback %q", forbidden)
		}
	}
}

func repositoryGoTestCall(timeout int) repositoryGoVerificationRequest {
	return repositoryGoVerificationRequest{
		Args:    []string{"test", "-json", "-count=1", "./..."},
		Timeout: time.Duration(timeout) * time.Second,
	}
}

func repositorySandboxFixture(
	t *testing.T,
	bubblewrapScript string,
) (string, repositoryWorkspaceProjection, repositoryGoSandboxConfig, *repositoryGoModuleView) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/sandbox\n\ngo 1.22\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	repositorySandboxGit(t, root, "init")
	repositorySandboxGit(t, root, "config", "user.name", "Omnidex Test")
	repositorySandboxGit(t, root, "config", "user.email", "omnidex@example.invalid")
	repositorySandboxGit(t, root, "add", "go.mod")
	repositorySandboxGit(t, root, "commit", "-m", "fixture")
	snapshot, err := repositoryfacts.BuildGitSnapshot(
		context.Background(), root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := newRepositorySnapshotProjection(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	tree := t.TempDir()
	toolchain := filepath.Join(tree, "toolchain")
	cache := filepath.Join(tree, "module-cache")
	if err := os.MkdirAll(filepath.Join(toolchain, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolchain, "bin", "go"), []byte("go"), 0o700); err != nil {
		t.Fatal(err)
	}
	bubblewrap := filepath.Join(tree, "bwrap")
	if err := os.WriteFile(bubblewrap, []byte(bubblewrapScript), 0o700); err != nil {
		t.Fatal(err)
	}
	config := repositoryGoSandboxConfig{
		BubblewrapPath: bubblewrap, GoRoot: toolchain, ModuleCache: cache,
	}
	view, err := projectRepositoryGoModuleView(context.Background(), root, cache, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = view.Cleanup() })
	return root, projection, config, view
}

func repositorySandboxGit(t *testing.T, root string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
}

func fakeBubblewrapScript(exitCode int, mutation string, delaySeconds int) string {
	if delaySeconds > 0 {
		return "#!/bin/sh\nexec sleep " + fmt.Sprint(delaySeconds) + "\n"
	}
	return "#!/bin/sh\n" +
		"args_fd=\"$2\"\n" +
		"info_fd=$(eval \"tr '\\\\000' '\\\\n' <&$args_fd\" | awk 'found { print; exit } $0 == \"--info-fd\" { found=1 }')\n" +
		"eval \"printf '%s' '{\\\"sandbox\\\":true}' >&$info_fd\"\n" +
		mutation + "\n" +
		"printf 'ok\\texample.com/sandbox\\t0.001s\\n'\n" +
		"printf 'exact stderr\\n' >&2\n" +
		"exit " + fmt.Sprint(exitCode) + "\n"
}
