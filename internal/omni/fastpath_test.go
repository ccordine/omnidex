package omni

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunFastPathDetectsPackageManager(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "package.json"), []byte(`{"scripts":{"build":"webpack"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	result := RunFastPath(context.Background(), "package.manager", workspace)
	if !result.Success {
		t.Fatalf("fastpath failed: %#v", result)
	}
	if result.Evidence["package_manager"] != "npm" {
		t.Fatalf("package manager = %q", result.Evidence["package_manager"])
	}
}

func TestOmniFastPathProjectProbeCommand(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader(""), &out, &errOut)
	err := app.Run([]string{"fastpath", "project.probe", "--workspace", workspace})
	if err != nil {
		t.Fatalf("fastpath failed: %v\nstderr=%s", err, errOut.String())
	}
	if !strings.Contains(out.String(), "go test ./...") {
		t.Fatalf("fastpath output = %q", out.String())
	}
}
