package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
