package omni

import (
	"strings"
	"testing"
)

func TestExternalAgentResultErrorDetectsCursorStatusError(t *testing.T) {
	result := CursorArchitectAgentResult{
		Output: `{"type":"status","agent_id":"agent-1","run_id":"run-1","status":"ERROR"}`,
	}
	err := externalAgentResultError(result)
	if err == nil {
		t.Fatal("expected error for cursor status ERROR")
	}
	if !strings.Contains(err.Error(), "run_id=run-1") || !strings.Contains(err.Error(), "agent_id=agent-1") {
		t.Fatalf("expected run and agent ids in error, got %v", err)
	}
}

func TestExternalAgentResultErrorDetectsTypedErrorEvent(t *testing.T) {
	result := CursorArchitectAgentResult{
		Output: `{"agent":"cursor","type":"error","message":"Cursor startup failed: invalid api key"}`,
	}
	if err := externalAgentResultError(result); err == nil {
		t.Fatal("expected error event to fail")
	}
}

func TestExternalAgentResultErrorDetectsLaunchFailure(t *testing.T) {
	result := CursorArchitectAgentResult{
		Output: `{"agent":"cursor","type":"error","message":"Cursor agent failed to launch: code=unauthenticated"}`,
	}
	if err := externalAgentResultError(result); err == nil {
		t.Fatal("expected launch failure to fail")
	}
}

func TestExternalAgentResultErrorDoesNotInferStatusFromPlainText(t *testing.T) {
	result := CursorArchitectAgentResult{
		Output: "Error: spawn codex ENOENT\nNode.js v25.2.1 (exit status 1)",
	}
	if err := externalAgentResultError(result); err != nil {
		t.Fatalf("plain text is not an authoritative status event: %v", err)
	}
}

func TestExternalAgentResultErrorIgnoresSuccess(t *testing.T) {
	result := CursorArchitectAgentResult{
		Output:  `{"agent":"cursor","type":"completed","message":"done"}`,
		Summary: "done",
	}
	if err := externalAgentResultError(result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
