package queue

import (
	"os"
	"strings"
	"testing"
)

func TestLifecycleMutationsLockGlobalIdentityBeforeAggregateAuthority(t *testing.T) {
	t.Parallel()
	cases := []struct {
		file, function, aggregate string
	}{
		{"repository_steps.go", "func (r *Repository) CompleteStep", "stepJobIDTx("},
		{"repository_steps.go", "func (r *Repository) FailStep", "stepJobIDTx("},
		{"repository_step_input.go", "func (r *Repository) SubmitJobFeedback", "submitJobFeedbackTx("},
		{"repository_replan.go", "func (r *Repository) ReplanJob", "replanJobTx("},
		{"repository_cancel.go", "func cancelJobTx", "lockedJobTx("},
		{"scrum_channel_operation_execute.go", "func (r *Repository) ExecuteScrumChannelOperation", "reserveLifecycleOperationIdentityTx("},
		{"scrum_channel_operation_execute.go", "func executeScrumChannelReplanTx", "replanJobTx("},
		{"scrum_channel_operation_execute.go", "func executeScrumChannelFeedbackTx", "submitJobFeedbackTx("},
	}
	for _, testCase := range cases {
		t.Run(testCase.function, func(t *testing.T) {
			source := lifecycleFunctionSource(t, testCase.file, testCase.function)
			lockAt := strings.Index(source, "lockLifecycleOperationIdentityTx(")
			aggregateAt := strings.Index(source, testCase.aggregate)
			if lockAt < 0 || aggregateAt < 0 || lockAt >= aggregateAt {
				t.Fatalf("%s lock=%d aggregate=%d", testCase.function, lockAt, aggregateAt)
			}
		})
	}
	if source := lifecycleSourceFile(t, "ai_control.go"); strings.Contains(source, "ORDER BY id\n\t\tFOR UPDATE") {
		t.Fatal("global AI pause locks jobs before deriving and locking cancellation identities")
	}
}

func lifecycleFunctionSource(t *testing.T, path, function string) string {
	t.Helper()
	source := lifecycleSourceFile(t, path)
	start := strings.Index(source, function)
	if start < 0 {
		t.Fatalf("%s is missing %q", path, function)
	}
	rest := source[start+len(function):]
	if next := strings.Index(rest, "\nfunc "); next >= 0 {
		rest = rest[:next]
	}
	return function + rest
}

func lifecycleSourceFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
