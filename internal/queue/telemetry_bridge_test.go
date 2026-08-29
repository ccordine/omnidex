package queue

import "testing"

func TestShouldRecordTelemetryStepEvent(t *testing.T) {
	cases := []struct {
		event   string
		message string
		want    bool
	}{
		{"step_error", "boom", true},
		{"repository_analysis_failed", "snapshot changed", true},
		{"coding_stage_passed", "generated_blocks=4", true},
		{"coding_phase_changed", "phase=constructing", false},
		{"objective_worker_rejected", "subject=context_relevance", true},
		{"coding_fragment_repair_guidance_started", "block=feature.001 exact_failure=TypeError", true},
		{"coding_fragment_correction_started", "block=feature.001 exact_failure=TypeError", true},
		{"workspace_mutation_indeterminate", "operation=abc", true},
		{"run_completed", "job_id=1", true},
	}
	for _, tc := range cases {
		if got := shouldRecordTelemetryStepEvent(tc.event, tc.message); got != tc.want {
			t.Fatalf("shouldRecordTelemetryStepEvent(%q, %q) = %v, want %v", tc.event, tc.message, got, tc.want)
		}
	}
}
