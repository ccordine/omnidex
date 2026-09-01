package main

import "testing"

func TestRuntimeEventChangesJobStateIncludesImmediatePlanReviewAvailability(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind string
		want bool
	}{
		{kind: "step_start", want: true},
		{kind: "step_complete", want: true},
		{kind: "step_failed", want: true},
		{kind: "step_authority_lost", want: true},
		{kind: "step_canceled", want: true},
		{kind: "step_waiting_input", want: true},
		{kind: "coding_plan_review_ready", want: true},
		{kind: "step_output", want: false},
		{kind: "coding_plan_candidate", want: false},
		{kind: "", want: false},
	}
	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			t.Parallel()
			if got := runtimeEventChangesJobState(test.kind); got != test.want {
				t.Fatalf("runtimeEventChangesJobState(%q) = %t, want %t", test.kind, got, test.want)
			}
		})
	}
}
