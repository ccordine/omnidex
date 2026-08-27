package worker

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

func TestDeploymentLifecycleRunsStatelessAndStatefulProofsBeforeSeal(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		hasState bool
		want     []string
	}{
		{
			name: "stateless", want: []string{
				"transition:applying", "command:build", "command:initial_start",
				"observe:initial_observe", "command:restart", "command:restart_start",
				"observe:final_observe", "evidence", "seal", "complete",
			},
		},
		{
			name: "stateful", hasState: true, want: []string{
				"transition:applying", "command:build", "command:initial_start",
				"command:migrate", "observe:initial_observe", "command:state_write",
				"command:restart", "command:restart_start", "observe:final_observe",
				"command:state_read", "evidence", "seal", "complete",
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			prepared := deploymentLifecyclePreparedFixture(testCase.hasState)
			runtime := newDeploymentLifecycleRuntimeFixture()
			outcome, err := runDirectCodingDeploymentLifecycle(
				prepared, deploymentLifecycleVerificationFixture(), runtime,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(runtime.events, testCase.want) {
				t.Fatalf("events=%v\nwant=%v", runtime.events, testCase.want)
			}
			if outcome.OperationID != runtime.operationID || len(outcome.ReceiptSHA256) != 64 ||
				runtime.receipt.EndpointPort == 0 {
				t.Fatalf("outcome=%+v receipt=%+v", outcome, runtime.receipt)
			}
			for _, event := range runtime.events {
				if event == "command:state_reset" {
					t.Fatal("persistent lifecycle attempted destructive application-state reset")
				}
			}
		})
	}
}

func TestDeploymentLifecycleFailureTransitionsAreScopedAndLoud(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		failSlot     queue.GeneratedWorkloadDeploymentLifecycleSlot
		rollbackFail bool
		wantState    queue.GeneratedWorkloadDeploymentState
		wantRollback bool
	}{
		{name: "after side effect", failSlot: queue.GeneratedDeploymentSlotInitialStart, wantState: queue.GeneratedWorkloadDeploymentRolledBack, wantRollback: true},
		{name: "rollback failure", failSlot: queue.GeneratedDeploymentSlotBuild, rollbackFail: true, wantState: queue.GeneratedWorkloadDeploymentIndeterminate, wantRollback: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runtime := newDeploymentLifecycleRuntimeFixture()
			runtime.failSlot, runtime.rollbackFail = testCase.failSlot, testCase.rollbackFail
			_, err := runDirectCodingDeploymentLifecycle(
				deploymentLifecyclePreparedFixture(false), deploymentLifecycleVerificationFixture(), runtime,
			)
			if err == nil {
				t.Fatal("deployment failure was hidden")
			}
			if runtime.lastTransition.State != testCase.wantState {
				t.Fatalf("transition=%+v", runtime.lastTransition)
			}
			gotRollback := containsString(runtime.events, "command:rollback")
			if gotRollback != testCase.wantRollback || runtime.sealed {
				t.Fatalf("events=%v sealed=%t", runtime.events, runtime.sealed)
			}
		})
	}
}

func TestDeploymentLifecycleRejectsRestartIdentityDriftBeforeSeal(t *testing.T) {
	runtime := newDeploymentLifecycleRuntimeFixture()
	runtime.observations[1].SHA256 = strings.Repeat("9", 64)
	_, err := runDirectCodingDeploymentLifecycle(
		deploymentLifecyclePreparedFixture(false), deploymentLifecycleVerificationFixture(), runtime,
	)
	if err == nil || runtime.lastTransition.State != queue.GeneratedWorkloadDeploymentRolledBack || runtime.sealed {
		t.Fatalf("error=%v transition=%+v sealed=%t", err, runtime.lastTransition, runtime.sealed)
	}
}

func TestDeploymentLifecyclePreservesSealedRealityWhenCognitionPersistenceFails(t *testing.T) {
	runtime := newDeploymentLifecycleRuntimeFixture()
	runtime.completeErr = errors.New("task ledger unavailable")
	_, err := runDirectCodingDeploymentLifecycle(
		deploymentLifecyclePreparedFixture(false), deploymentLifecycleVerificationFixture(), runtime,
	)
	if err == nil || !runtime.sealed || containsString(runtime.events, "command:rollback") {
		t.Fatalf("error=%v events=%v sealed=%t", err, runtime.events, runtime.sealed)
	}
}

func TestDeploymentLifecycleSnapshotDriftBeforeBuildDoesNotSpawnRollback(t *testing.T) {
	runtime := newDeploymentLifecycleRuntimeFixture()
	runtime.failSlot = queue.GeneratedDeploymentSlotBuild
	runtime.failErr = fmt.Errorf("%w: mutated before build", errDirectCodingDeploymentSnapshotDrift)
	_, err := runDirectCodingDeploymentLifecycle(
		deploymentLifecyclePreparedFixture(false), deploymentLifecycleVerificationFixture(), runtime,
	)
	if err == nil || runtime.lastTransition.State != queue.GeneratedWorkloadDeploymentFailed ||
		containsString(runtime.events, "command:rollback") || runtime.sealed {
		t.Fatalf("error=%v events=%v transition=%+v sealed=%t", err, runtime.events, runtime.lastTransition, runtime.sealed)
	}
}

type deploymentLifecycleRuntimeFixture struct {
	events         []string
	observations   []directCodingDeploymentObservation
	observation    int
	failSlot       queue.GeneratedWorkloadDeploymentLifecycleSlot
	failErr        error
	rollbackFail   bool
	lastTransition queue.GeneratedWorkloadDeploymentTransition
	receipt        queue.GeneratedWorkloadDeploymentReceipt
	sealed         bool
	completeErr    error
	operationID    string
	now            time.Time
}

func newDeploymentLifecycleRuntimeFixture() *deploymentLifecycleRuntimeFixture {
	observation := deploymentLifecycleObservationFixture()
	return &deploymentLifecycleRuntimeFixture{
		observations: []directCodingDeploymentObservation{observation, observation},
		operationID:  "generated_workload_deployment_" + strings.Repeat("a", 64),
		now:          time.Unix(1_800_000_000, 123_456_000).UTC(),
	}
}

func (runtime *deploymentLifecycleRuntimeFixture) Transition(
	transition queue.GeneratedWorkloadDeploymentTransition,
) (queue.GeneratedWorkloadDeploymentRecord, error) {
	runtime.events = append(runtime.events, "transition:"+string(transition.State))
	runtime.lastTransition = transition
	return queue.GeneratedWorkloadDeploymentRecord{OperationID: runtime.operationID, State: transition.State}, nil
}

func (runtime *deploymentLifecycleRuntimeFixture) Execute(
	slot queue.GeneratedWorkloadDeploymentLifecycleSlot,
) error {
	runtime.events = append(runtime.events, "command:"+slot.Name)
	if slot == queue.GeneratedDeploymentSlotRollback && runtime.rollbackFail {
		return errors.New("rollback failed")
	}
	if slot == runtime.failSlot {
		if runtime.failErr != nil {
			return runtime.failErr
		}
		return errors.New("injected command failure")
	}
	return nil
}

func (runtime *deploymentLifecycleRuntimeFixture) Rollback(
	transition queue.GeneratedWorkloadDeploymentTransition,
) error {
	runtime.events = append(runtime.events, "command:rollback")
	if runtime.rollbackFail {
		runtime.lastTransition = queue.GeneratedWorkloadDeploymentTransition{
			State: queue.GeneratedWorkloadDeploymentIndeterminate,
		}
		return errors.New("rollback failed")
	}
	runtime.lastTransition = transition
	return nil
}

func (runtime *deploymentLifecycleRuntimeFixture) Observe(
	slot queue.GeneratedWorkloadDeploymentLifecycleSlot,
) (directCodingDeploymentObservation, error) {
	runtime.events = append(runtime.events, "observe:"+slot.Name)
	result := runtime.observations[runtime.observation]
	runtime.observation++
	return result, nil
}

func (runtime *deploymentLifecycleRuntimeFixture) EvidenceIDs() ([]int64, []int64, error) {
	runtime.events = append(runtime.events, "evidence")
	return []int64{41, 42, 43, 44, 45, 46}, []int64{51, 52}, nil
}

func (runtime *deploymentLifecycleRuntimeFixture) Seal(
	receipt queue.GeneratedWorkloadDeploymentReceipt,
) (queue.GeneratedWorkloadDeploymentRecord, error) {
	runtime.events = append(runtime.events, "seal")
	runtime.receipt, runtime.sealed = receipt, true
	return queue.GeneratedWorkloadDeploymentRecord{
		OperationID: runtime.operationID, State: queue.GeneratedWorkloadDeploymentApplied,
		ReceiptSHA256: strings.Repeat("b", 64), EvidenceID: 51,
	}, nil
}

func (runtime *deploymentLifecycleRuntimeFixture) CompleteDeployment(string, string) error {
	runtime.events = append(runtime.events, "complete")
	return runtime.completeErr
}

func (runtime *deploymentLifecycleRuntimeFixture) Now() time.Time {
	value := runtime.now
	runtime.now = runtime.now.Add(time.Second)
	return value
}

func deploymentLifecyclePreparedFixture(hasState bool) directCodingPreparedDeployment {
	descriptor := *genericPHPDeploymentDescriptor()
	if !hasState {
		descriptor.MigrationScript = ""
	}
	return directCodingPreparedDeployment{
		descriptor: descriptor, hasState: hasState,
		verification: queue.GeneratedWorkloadVerificationRecord{
			ID: "generated_workload_verification_" + strings.Repeat("9", 64),
		},
		command: queue.GeneratedWorkloadDeploymentCommand{
			ComposeProject: "omnidex-job-7-g1", ConfigSHA256: strings.Repeat("c", 64),
		},
	}
}

func deploymentLifecycleVerificationFixture() directCodingVerification {
	return directCodingVerification{
		Passed: true, TestsPassed: true, Commands: []string{"verify"}, EvidenceIDs: []int64{31},
		MutationOperationID: "workspace_mutation_" + strings.Repeat("c", 64), MutationReceiptSHA256: strings.Repeat("d", 64),
	}
}

func deploymentLifecycleObservationFixture() directCodingDeploymentObservation {
	return directCodingDeploymentObservation{
		Schema: directCodingDeploymentObservationSchema, Project: "omnidex-job-7-g1",
		Services: []directCodingObservedService{{
			Service: "app", ContainerID: strings.Repeat("d", 64),
			ImageID: "sha256:" + strings.Repeat("e", 64), RestartPolicy: "unless-stopped",
			State: "running", Health: "healthy",
		}},
		Endpoint: directCodingObservedEndpoint{
			Scheme: "http", Host: "service.example.test", Port: 18080,
			Path: directCodingDeploymentReadinessPath,
		},
		SHA256: strings.Repeat("f", 64),
	}
}
