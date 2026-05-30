package queue

import "testing"

func TestShouldRecordTelemetryStepEvent(t *testing.T) {
	cases := []struct {
		event   string
		message string
		want    bool
	}{
		{"step_error", "boom", true},
		{"verify_auto_replan", "attempt=2", true},
		{"verify_test_pass", "cmd=go test", true},
		{"plan_begin", "autonomy=full", false},
		{"llm_prompt", "scope=plan chars=12000", true},
		{"llm_retry_model", "scope=plan from=a to=b", true},
		{"verify_ready", "status=pass attempted=1 failed=0", false},
		{"verify_ready", "status=blocked attempted=1 failed=0", true},
		{"verify_ready", "status=pass attempted=2 failed=1", true},
	}
	for _, tc := range cases {
		if got := shouldRecordTelemetryStepEvent(tc.event, tc.message); got != tc.want {
			t.Fatalf("shouldRecordTelemetryStepEvent(%q, %q) = %v, want %v", tc.event, tc.message, got, tc.want)
		}
	}
}
