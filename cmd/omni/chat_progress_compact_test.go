package main

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestNormalCommandProgressDoesNotFloodConversation(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console}
	activity := nativeRenderActivity(model.AgentActivityCommand, model.AgentActivityStarted, "")
	activity.Command = "python3 - <<'PY'\n" + strings.Repeat("large script body\n", 200) + "PY"
	if err := renderer.realtime(nativeRenderEvent(activity)); err != nil {
		t.Fatal(err)
	}
	zero := 0
	activity.Status = model.AgentActivityCompleted
	activity.ExitCode = &zero
	if err := renderer.realtime(nativeRenderEvent(activity)); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("routine commands flooded conversation: %s", output.String())
	}
	one := 1
	activity.Status = model.AgentActivityFailed
	activity.ExitCode = &one
	if err := renderer.realtime(nativeRenderEvent(activity)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Command exited 1") || strings.Contains(output.String(), "large script body") || strings.Contains(output.String(), "Reviewing · failed") {
		t.Fatalf("misleading command outcome: %s", output.String())
	}
}
