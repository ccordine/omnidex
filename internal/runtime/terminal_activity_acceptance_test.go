package runtime

import (
	"strings"
	"testing"
)

// Keep observed evidence kinds across the bounded transcript window, without
// retaining a growing native/model output log or relying on private wording.
type terminalWorkflowActivity struct {
	seen          map[string]bool
	tail          string
	requireOutput bool
}

func (activity *terminalWorkflowActivity) observe(text string) {
	if activity.seen == nil {
		activity.seen = make(map[string]bool)
	}
	text = activity.tail + text
	for _, marker := range []string{"Collect requirements · started", "Collect requirements · completed", "thinking>", "command>", "exit 0", "file read", "read ·", "list ·", "output>", "Written files:", "Planning · started", "Executing task · started", "Reviewing · started", "Overall review · started", "Overall review · completed"} {
		activity.seen[marker] = activity.seen[marker] || strings.Contains(text, marker)
	}
	if len(text) > 128 {
		text = text[len(text)-128:]
	}
	activity.tail = text
}

func (activity *terminalWorkflowActivity) assert(t *testing.T) {
	t.Helper()
	for _, marker := range []string{"thinking>", "command>", "exit 0", "list ·", "Written files:", "Planning · started", "Executing task · started", "Reviewing · started", "Overall review · started", "Overall review · completed"} {
		if !activity.seen[marker] {
			t.Errorf("actual terminal workflow omitted evidence %q", marker)
		}
	}
	if !activity.seen["file read"] && !activity.seen["read ·"] {
		t.Error("actual terminal workflow omitted file reads")
	}
	if activity.requireOutput && !activity.seen["output>"] {
		t.Error("verbose terminal workflow omitted actual command output")
	}
}
