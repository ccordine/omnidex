package worker

import (
	"os"
	"strings"
	"testing"
)

func TestNativeRuntimeCompletionUsesTerminalNotificationPath(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("runtime_v3_artifact_store.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"r.svc.completeStep(r.ctx, command)",
		"r.svc.notifyJobFinishedForStep(r.ctx, command.StepID)",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("native terminal completion omits %q", required)
		}
	}
	if strings.Contains(source, "r.svc.repo.CompleteStep(r.ctx, command)") {
		t.Fatal("native completion bypasses the worker terminal notification path")
	}
}
