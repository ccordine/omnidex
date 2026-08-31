package main

import (
	"strings"
	"testing"
)

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

func TestTracedExecCommandEmitsTrace(t *testing.T) {
	var seen []string
	restore := installLocalExecutionTraceSink(func(line string) {
		seen = append(seen, line)
	})
	defer restore()

	if err := tracedExecCommand("pwd").Run(); err != nil {
		t.Fatalf("traced command: %v", err)
	}
	if len(seen) == 0 {
		t.Fatal("expected at least one trace line")
	}
	joined := strings.Join(seen, "\n")
	if !strings.Contains(joined, "pwd") {
		t.Fatalf("expected traced command to mention pwd, got %q", joined)
	}
}
