package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecuteV3CommandAtRootUsesExplicitServerDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/stage\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "stage_test.go"), []byte(`package stage

import "testing"

func TestStage(t *testing.T) {}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := executeCodeCommandAtRoot(context.Background(), root, codeCommand{
		Program: "go", Args: []string{"test", "./..."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if succeeded, _ := result.Output["succeeded"].(bool); !succeeded {
		t.Fatalf("staged command failed: %+v", result.Output)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].Metadata["workspace"] != root {
		t.Fatalf("staged command evidence=%+v", result.Evidence)
	}
}

func TestExecuteV3CommandAtRootRejectsUnboundRoot(t *testing.T) {
	t.Parallel()
	_, err := executeCodeCommandAtRoot(context.Background(), "relative", codeCommand{
		Program: "go", Args: []string{"version"},
	})
	if err == nil {
		t.Fatal("command.run accepted a relative unbound root")
	}
}

func TestGeneratedCommandEnvironmentExcludesServerSecretsAndHome(t *testing.T) {
	parentHome := os.Getenv("HOME")
	t.Setenv("OMNIDEX_PRIVATE_TOKEN", "must-not-enter-generated-process")
	root := writeCommandScopeGoFixture(t, fmt.Sprintf(`package stage

import (
	"os"
	"testing"
)

func TestEnvironment(t *testing.T) {
	if os.Getenv("OMNIDEX_PRIVATE_TOKEN") != "" { t.Fatal("inherited server secret") }
	if os.Getenv("HOME") == %q { t.Fatal("inherited server home") }
}
`, parentHome))
	output, err := runDirectCodingStageCommand(
		context.Background(), root, defaultV3CommandLimit, "go", "test", "./...",
	)
	if err != nil {
		t.Fatalf("isolated generated command failed: %v\n%s", err, output)
	}
}

func TestGeneratedCommandOutputIsBounded(t *testing.T) {
	root := writeCommandScopeGoFixture(t, `package stage

import (
	"fmt"
	"strings"
	"testing"
)

func TestOutput(t *testing.T) {
	fmt.Print(strings.Repeat("x", 50000))
	t.Fatal("expected failure")
}
`)
	output, err := runDirectCodingStageCommand(
		context.Background(), root, defaultV3CommandLimit, "go", "test", "./...",
	)
	if err == nil {
		t.Fatal("expected generated test failure")
	}
	if len(output) > 2*maxV3CommandOutput+1 || !strings.HasPrefix(output, "x") {
		t.Fatalf("generated command output was not bounded: bytes=%d", len(output))
	}
}

func TestGeneratedCommandRejectsSymlinkedRootAncestor(t *testing.T) {
	base := t.TempDir()
	realParent := filepath.Join(base, "real")
	realRoot := filepath.Join(realParent, "root")
	if err := os.MkdirAll(realRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(realParent, alias); err != nil {
		t.Fatal(err)
	}
	_, err := runDirectCodingStageCommand(
		context.Background(), filepath.Join(alias, "root"), defaultV3CommandLimit,
		"go", "version",
	)
	if err == nil {
		t.Fatal("generated command accepted a workspace beneath a symbolic-link ancestor")
	}
}

func TestGeneratedCommandFailsOnTimeoutAndRelativePathAuthority(t *testing.T) {
	root := writeCommandScopeGoFixture(t, "package stage\n")
	_, err := runDirectCodingStageCommand(
		context.Background(), root, time.Nanosecond, "go", "version",
	)
	if err == nil || !strings.Contains(err.Error(), "command exceeded") {
		t.Fatalf("generated command timeout error=%v", err)
	}
	t.Setenv("PATH", "."+string(os.PathListSeparator)+"/usr/bin")
	_, err = runDirectCodingStageCommand(
		context.Background(), root, defaultV3CommandLimit, "go", "version",
	)
	if err == nil || !strings.Contains(err.Error(), "exact absolute directories") {
		t.Fatalf("generated command relative PATH error=%v", err)
	}
}

func writeCommandScopeGoFixture(t *testing.T, source string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/stage\n\ngo 1.24\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "stage_test.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}
