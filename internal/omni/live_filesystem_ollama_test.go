package omni

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveOllamaCreatesRequestedProjectDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live filesystem command test in short mode")
	}
	skipUnlessLiveOllamaEnabled(t)
	client := testOllamaClient(t)
	client.Client.Timeout = 2 * time.Minute

	root := t.TempDir()
	target := filepath.Join(root, "odin-antigravity-game")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	result, err := RunStructuredCommandDecision(ctx, "In "+root+", create a test project directory called odin-antigravity-game.", client, stdout, stderr)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRealCommandObservation(result.Observations) {
		t.Fatalf("expected real command observation: %#v", result.Observations)
	}
	assertNoFalseCapabilityLimitation(t, client, result, stdout.String(), stderr.String())
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("expected target directory %s to exist; command=%q answer=%q stdout=%q stderr=%q err=%v", target, result.Command, result.Answer, stdout.String(), stderr.String(), err)
	}
	if !info.IsDir() {
		t.Fatalf("target exists but is not directory: %s", target)
	}
}
