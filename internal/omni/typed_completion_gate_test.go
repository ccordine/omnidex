package omni

import "testing"

func TestRejectCompletionCheckerWithoutTypedWorkQueueBlocksNLAcceptance(t *testing.T) {
	result := &CommandDecisionResult{
		WorkItems: []ObjectiveWorkItem{{
			ID:     "create_notes_app",
			Kind:   WorkItemKindCreate,
			Status: WorkItemStatusPassed,
		}},
		Answer: "The notes app is complete.",
	}
	if !rejectCompletionCheckerWithoutTypedWorkQueue(3, nil, result) {
		t.Fatal("completion checker must be rejected when typed queue lacks evidence")
	}
	if result.Answer != "" {
		t.Fatalf("answer should be cleared, got %q", result.Answer)
	}
}

func TestRejectCompletionCheckerWithoutTypedWorkQueueAllowsCompleteQueue(t *testing.T) {
	result := &CommandDecisionResult{
		WorkItems: []ObjectiveWorkItem{
			passingWorkItem("verify_build", WorkItemKindVerify, EvidenceKindCommand),
		},
		Answer: "Build verified.",
	}
	if rejectCompletionCheckerWithoutTypedWorkQueue(3, nil, result) {
		t.Fatal("completion checker should proceed when typed queue fully passed")
	}
}

func TestRejectCompletionCheckerWithoutTypedWorkQueueIgnoresEmptyQueue(t *testing.T) {
	result := &CommandDecisionResult{Answer: "legacy completion path"}
	if rejectCompletionCheckerWithoutTypedWorkQueue(1, nil, result) {
		t.Fatal("empty typed queue should not block legacy completion checker path")
	}
}
