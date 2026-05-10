package main

import (
	"strings"
	"testing"
	"time"
)

func TestFormatLocalAutomationTraceIncludesDetails(t *testing.T) {
	candidate := &chatActionCandidate{
		Kind:         "local_shell",
		Summary:      "create file `demo.html` in the current directory",
		SpecialistID: "shell_execution_specialist",
	}

	got := formatLocalAutomationTrace(candidate)
	for _, want := range []string{
		"frontend action",
		"kind=local_shell",
		"specialist=shell_execution_specialist",
		`summary="create file ` + "`demo.html`" + ` in the current directory"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("trace missing %q: %q", want, got)
		}
	}
}

func TestFormatLocalReviewHandoffTraceIncludesCoreTarget(t *testing.T) {
	candidate := &chatActionCandidate{
		Kind:         "local_shell",
		SpecialistID: "shell_execution_specialist",
	}
	got := formatLocalReviewHandoffTrace(candidate, "Executed: touch demo.html")
	for _, want := range []string{
		"frontend handoff",
		"target=core",
		"phase=deterministic_local_action_review",
		"kind=local_shell",
		"specialist=shell_execution_specialist",
		"output_chars=",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("handoff trace missing %q: %q", want, got)
		}
	}
}

func TestTraceLocalCommandInvocationUsesInstalledSink(t *testing.T) {
	var seen []string
	restore := installLocalExecutionTraceSink(func(line string) {
		seen = append(seen, line)
	})
	defer restore()

	traceLocalCommandInvocation("touch", "demo.html")
	if len(seen) != 1 {
		t.Fatalf("trace count=%d want 1", len(seen))
	}
	if !strings.Contains(seen[0], "frontend exec") || !strings.Contains(seen[0], "touch demo.html") {
		t.Fatalf("unexpected trace line: %q", seen[0])
	}
}

func TestRunLocalCommandEmitsTrace(t *testing.T) {
	var seen []string
	restore := installLocalExecutionTraceSink(func(line string) {
		seen = append(seen, line)
	})
	defer restore()

	if _, err := runLocalCommand([]string{"pwd"}, time.Second); err != nil {
		t.Fatalf("runLocalCommand: %v", err)
	}
	if len(seen) == 0 {
		t.Fatal("expected at least one trace line")
	}
	joined := strings.Join(seen, "\n")
	if !strings.Contains(joined, "pwd") {
		t.Fatalf("expected traced command to mention pwd, got %q", joined)
	}
}
