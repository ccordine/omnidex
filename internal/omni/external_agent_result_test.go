package omni

import "testing"

func TestExternalAgentResultErrorDetectsCursorStatusError(t *testing.T) {
	result := CursorArchitectAgentResult{
		Output: `{"type":"status","agent_id":"agent-1","run_id":"run-1","status":"ERROR"}`,
	}
	if err := externalAgentResultError(result); err == nil {
		t.Fatal("expected error for cursor status ERROR")
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

func TestExternalAgentResultErrorIgnoresSuccess(t *testing.T) {
	result := CursorArchitectAgentResult{
		Output: `{"agent":"cursor","type":"completed","message":"done"}`,
		Summary: "done",
	}
	if err := externalAgentResultError(result); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
