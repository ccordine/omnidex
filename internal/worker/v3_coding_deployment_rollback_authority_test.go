package worker

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestDeploymentRollbackWithoutInitialStartCannotReachJournalOrCommand(t *testing.T) {
	t.Parallel()
	observerErr := errors.New("Docker observation unavailable")
	for _, testCase := range []struct {
		name         string
		needsCommand bool
		observeErr   error
		want         string
	}{
		{name: "clean", want: "stopped without a terminal result"},
		{name: "residual", needsCommand: true, want: "forbidden before a durable initial_start"},
		{name: "observer error", observeErr: observerErr, want: observerErr.Error()},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			observeCalls, beginCalls, executorCalls := 0, 0, 0
			proceed, err := authorizeDirectCodingDestructiveRollback(false, func() (bool, error) {
				observeCalls++
				return testCase.needsCommand, testCase.observeErr
			})
			if proceed {
				beginCalls++
				executorCalls++
			}
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("pre-start rollback error=%v", err)
			}
			if observeCalls != 1 || beginCalls != 0 || executorCalls != 0 {
				t.Fatalf(
					"observers=%d rollback begins=%d command executors=%d want 1/0/0",
					observeCalls, beginCalls, executorCalls,
				)
			}
		})
	}
}

func TestLateRecoveryRestoresInitialStartOwnershipBeforeRollbackEligibility(t *testing.T) {
	t.Parallel()
	runtime := &directCodingSessionDeploymentRuntime{}
	evidence := &queue.GeneratedWorkloadDeploymentEvidenceSnapshot{
		Executions: []queue.GeneratedWorkloadDeploymentExecutionRecord{
			{Slot: queue.GeneratedDeploymentSlotBuild, Status: queue.GeneratedWorkloadDeploymentExecutionCompleted},
			{Slot: queue.GeneratedDeploymentSlotInitialStart, Status: queue.GeneratedWorkloadDeploymentExecutionCompleted},
		},
	}
	if err := restoreDirectCodingRecoveredExecutions(runtime, evidence); err != nil {
		t.Fatal(err)
	}
	observeCalls := 0
	proceed, err := authorizeDirectCodingDestructiveRollback(
		runtime.hasDurableInitialStartExecution(),
		func() (bool, error) {
			observeCalls++
			return false, nil
		},
	)
	if err != nil || !proceed || observeCalls != 0 {
		t.Fatalf("late recovery rollback eligibility=%t observer calls=%d err=%v", proceed, observeCalls, err)
	}
}

func TestNamespaceQualificationInfrastructureFailuresReachNoRollbackExecutor(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name string
		slot queue.GeneratedWorkloadDeploymentLifecycleSlot
		err  error
	}{
		{name: "Docker observer", slot: queue.GeneratedDeploymentSlotBuild, err: errors.New("Docker observer timeout")},
		{name: "proof repository", slot: queue.GeneratedDeploymentSlotInitialStart, err: errors.New("record namespace proof: database unavailable")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			base := newDeploymentLifecycleRuntimeFixture()
			base.failSlot, base.failErr = testCase.slot, testCase.err
			runtime := &preStartOwnershipLifecycleRuntime{deploymentLifecycleRuntimeFixture: base}
			_, err := runDirectCodingDeploymentLifecycle(
				deploymentLifecyclePreparedFixture(false),
				deploymentLifecycleVerificationFixture(), runtime,
			)
			if err == nil || runtime.rollbackCalls != 1 || runtime.observerCalls != 1 ||
				runtime.rollbackBeginCalls != 0 || runtime.rollbackExecutorCalls != 0 {
				t.Fatalf(
					"qualification error=%v rollback/observer/begin/executor=%d/%d/%d/%d",
					err, runtime.rollbackCalls, runtime.observerCalls,
					runtime.rollbackBeginCalls, runtime.rollbackExecutorCalls,
				)
			}
			if _, typed := directCodingDeploymentNamespaceFailureAuthority(testCase.err); typed {
				t.Fatal("infrastructure failure was mislabeled as a proven namespace conflict")
			}
		})
	}
}

type preStartOwnershipLifecycleRuntime struct {
	*deploymentLifecycleRuntimeFixture
	rollbackCalls         int
	observerCalls         int
	rollbackBeginCalls    int
	rollbackExecutorCalls int
}

func (runtime *preStartOwnershipLifecycleRuntime) Rollback(
	queue.GeneratedWorkloadDeploymentTransition,
) error {
	runtime.rollbackCalls++
	proceed, err := authorizeDirectCodingDestructiveRollback(false, func() (bool, error) {
		runtime.observerCalls++
		return false, errors.New("Docker observer unavailable")
	})
	if proceed {
		runtime.rollbackBeginCalls++
		runtime.rollbackExecutorCalls++
	}
	return err
}
