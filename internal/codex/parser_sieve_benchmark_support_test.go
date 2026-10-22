package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func installedAgentClient(t *testing.T) (*Client, string) {
	t.Helper()
	command, model := os.Getenv("OMNI_TEST_CODEX_COMMAND"), os.Getenv("OMNI_TEST_CODEX_MODEL")
	if command == "" || model == "" {
		t.Skip("installed Codex parser benchmark not selected")
	}
	directory, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(command, directory)
	if err != nil {
		t.Fatal(err)
	}
	return client, model
}
