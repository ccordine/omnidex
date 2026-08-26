package worker

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestDirectCodingDeploymentRecoveryRoutesEveryDurableState(t *testing.T) {
	tests := []struct {
		name        string
		snapshot    *queue.GeneratedWorkloadDeploymentSnapshot
		disposition directCodingCompletionTaskDisposition
		want        string
		wantError   string
	}{
		{name: "before journal", disposition: directCodingCompletionTaskResumed, want: "ordinary"},
		{name: "prepared", snapshot: recoverySnapshot(queue.GeneratedWorkloadDeploymentPrepared, false), disposition: directCodingCompletionTaskResumed, want: "resume"},
		{name: "applying", snapshot: recoverySnapshot(queue.GeneratedWorkloadDeploymentApplying, false), disposition: directCodingCompletionTaskResumed, want: "resume"},
		{name: "indeterminate", snapshot: recoverySnapshot(queue.GeneratedWorkloadDeploymentIndeterminate, false), disposition: directCodingCompletionTaskResumed, want: "resume"},
		{name: "applied active cognition", snapshot: recoverySnapshot(queue.GeneratedWorkloadDeploymentApplied, true), disposition: directCodingCompletionTaskResumed, wantError: "pre-workspace gate"},
		{name: "applied completed cognition", snapshot: recoverySnapshot(queue.GeneratedWorkloadDeploymentApplied, true), disposition: directCodingCompletionTaskAlreadyDone, wantError: "pre-workspace gate"},
		{name: "failed", snapshot: recoverySnapshot(queue.GeneratedWorkloadDeploymentFailed, false), disposition: directCodingCompletionTaskResumed, wantError: "terminal"},
		{name: "rolled back", snapshot: recoverySnapshot(queue.GeneratedWorkloadDeploymentRolledBack, false), disposition: directCodingCompletionTaskResumed, wantError: "terminal"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			backend := &directCodingDeploymentRecoveryBackendFixture{snapshot: testCase.snapshot}
			recovery := &directCodingDeploymentRecovery{backend: backend}
			_, err := recovery.RecoverVerifiedDeployment(
				recoveryVerification(), testCase.disposition,
			)
			if testCase.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantError) {
					t.Fatalf("error=%v", err)
				}
				if backend.totalEffects() != 0 {
					t.Fatalf("terminal recovery effects=%+v", backend)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if backend.route() != testCase.want {
				t.Fatalf("route=%q backend=%+v", backend.route(), backend)
			}
		})
	}
}

func TestLateAppliedDeploymentRecoveryFailsWithoutFallbackEffects(t *testing.T) {
	backend := &directCodingDeploymentRecoveryBackendFixture{
		snapshot: recoverySnapshot(queue.GeneratedWorkloadDeploymentApplied, true),
	}
	recovery := &directCodingDeploymentRecovery{backend: backend}
	_, err := recovery.RecoverVerifiedDeployment(
		recoveryVerification(), directCodingCompletionTaskResumed,
	)
	if err == nil || !strings.Contains(err.Error(), "pre-workspace gate") {
		t.Fatalf("late applied recovery error=%v", err)
	}
	if backend.totalEffects() != 0 {
		t.Fatalf("late applied recovery invoked fallback effects: %+v", backend)
	}
}

func TestDeploymentRecoveryRejectsInconsistentJournalAndCognition(t *testing.T) {
	withReceipt := recoverySnapshot(queue.GeneratedWorkloadDeploymentPrepared, true)
	tests := []struct {
		name        string
		snapshot    *queue.GeneratedWorkloadDeploymentSnapshot
		disposition directCodingCompletionTaskDisposition
	}{
		{name: "done without journal", disposition: directCodingCompletionTaskAlreadyDone},
		{name: "prepared with receipt", snapshot: withReceipt, disposition: directCodingCompletionTaskResumed},
		{name: "prepared done cognition", snapshot: recoverySnapshot(queue.GeneratedWorkloadDeploymentPrepared, false), disposition: directCodingCompletionTaskAlreadyDone},
		{name: "applied without receipt", snapshot: recoverySnapshot(queue.GeneratedWorkloadDeploymentApplied, false), disposition: directCodingCompletionTaskResumed},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			backend := &directCodingDeploymentRecoveryBackendFixture{snapshot: testCase.snapshot}
			recovery := &directCodingDeploymentRecovery{backend: backend}
			if _, err := recovery.RecoverVerifiedDeployment(
				recoveryVerification(), testCase.disposition,
			); err == nil {
				t.Fatal("inconsistent recovery state succeeded")
			}
			if backend.totalEffects() != 0 {
				t.Fatalf("inconsistent recovery effects=%+v", backend)
			}
		})
	}
}

func TestEveryProductionDirectCodingSessionRegistersDeploymentRecovery(t *testing.T) {
	source, err := os.ReadFile("v3_coding_runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(source), "session.deploymentRecovery = newDirectCodingDeploymentRecovery(session)") != 1 {
		t.Fatal("production direct-coding session lacks one concrete deployment recovery registration")
	}
}

func recoveryVerification() directCodingVerification {
	return directCodingVerification{
		Passed: true, TestsPassed: true,
		Commands: []string{"go test ./..."}, EvidenceIDs: []int64{1},
	}
}

func recoverySnapshot(
	state queue.GeneratedWorkloadDeploymentState,
	receipt bool,
) *queue.GeneratedWorkloadDeploymentSnapshot {
	snapshot := &queue.GeneratedWorkloadDeploymentSnapshot{
		Record: queue.GeneratedWorkloadDeploymentRecord{
			OperationID: "operation", State: state,
			ReceiptSHA256: strings.Repeat("a", 64), EvidenceID: 1,
		},
	}
	if receipt {
		snapshot.Receipt = &queue.GeneratedWorkloadDeploymentReceipt{}
	}
	return snapshot
}

type directCodingDeploymentRecoveryBackendFixture struct {
	snapshot         *queue.GeneratedWorkloadDeploymentSnapshot
	loadErr          error
	ordinary, resume int
}

func (fixture *directCodingDeploymentRecoveryBackendFixture) CurrentDeployment() (*queue.GeneratedWorkloadDeploymentSnapshot, error) {
	return fixture.snapshot, fixture.loadErr
}

func (fixture *directCodingDeploymentRecoveryBackendFixture) DeployBeforeJournal(directCodingVerification) (directCodingDeploymentOutcome, error) {
	fixture.ordinary++
	return directCodingDeploymentOutcome{}, nil
}

func (fixture *directCodingDeploymentRecoveryBackendFixture) ResumeJournaledDeployment(*queue.GeneratedWorkloadDeploymentSnapshot, directCodingVerification) (directCodingDeploymentOutcome, error) {
	fixture.resume++
	return directCodingDeploymentOutcome{}, nil
}

func (fixture *directCodingDeploymentRecoveryBackendFixture) totalEffects() int {
	return fixture.ordinary + fixture.resume
}

func (fixture *directCodingDeploymentRecoveryBackendFixture) route() string {
	switch {
	case fixture.ordinary == 1:
		return "ordinary"
	case fixture.resume == 1:
		return "resume"
	default:
		return ""
	}
}
