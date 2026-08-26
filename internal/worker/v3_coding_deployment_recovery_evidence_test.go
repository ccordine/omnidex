package worker

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestAppliedRecoveryRequiresExactSuccessfulFinalEvidence(t *testing.T) {
	snapshot, evidence, expected := directCodingAppliedRecoveryEvidenceFixture()
	observed, err := directCodingRecoveredFinalObservation(snapshot, &evidence)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(observed, expected) {
		t.Fatalf("observed=%+v\nwant=%+v", observed, expected)
	}
	changed := evidence
	changed.Observations = append(
		[]queue.GeneratedWorkloadDeploymentObservationRecord(nil),
		evidence.Observations...,
	)
	changed.Observations[1].Observation.Services[0].ImageDigest =
		"sha256:" + strings.Repeat("9", 64)
	if _, err := directCodingRecoveredFinalObservation(snapshot, &changed); err == nil ||
		!strings.Contains(err.Error(), "differs") {
		t.Fatalf("changed final observation error=%v", err)
	}
	failed := false
	changed = evidence
	changed.Executions = append(
		[]queue.GeneratedWorkloadDeploymentExecutionRecord(nil),
		evidence.Executions...,
	)
	changed.Executions[0].Succeeded = &failed
	if _, err := directCodingRecoveredFinalObservation(snapshot, &changed); err == nil ||
		!strings.Contains(err.Error(), "successful manifest") {
		t.Fatalf("failed execution error=%v", err)
	}
}

func TestRecoveryRuntimeRefusesBlindReplayOfStartedSideEffectSlot(t *testing.T) {
	source, err := os.ReadFile("v3_coding_deployment_runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	start := strings.Index(text, "func (runtime *directCodingSessionDeploymentRuntime) execute(")
	if start < 0 {
		t.Fatal("deployment runtime execution function is unavailable")
	}
	end := strings.Index(text[start:], "func (runtime *directCodingSessionDeploymentRuntime) Observe(")
	if end < 0 {
		t.Fatal("deployment runtime execution function is unavailable")
	}
	body := text[start : start+end]
	replay := strings.Index(body, "record.Status == queue.GeneratedWorkloadDeploymentExecutionCompleted")
	refusal := strings.Index(body, "may have produced a side effect and requires reconciliation")
	execute := strings.Index(body, "executeDirectCodingDeploymentCommand(")
	if replay < 0 || refusal <= replay || execute <= refusal {
		t.Fatal("deployment runtime can execute before reconciling a durable started slot")
	}
}

func TestStartedForwardDetailIsIdenticalAcrossEveryObservationFence(t *testing.T) {
	t.Parallel()
	started := queue.GeneratedWorkloadDeploymentExecutionRecord{
		OperationID: "generated_workload_deployment_" + strings.Repeat("a", 64),
		Slot:        queue.GeneratedDeploymentSlotBuild,
		StepAttempt: 7, WorkerID: "worker-seven",
		CommandSHA256: strings.Repeat("b", 64), WorkspaceSHA256: strings.Repeat("c", 64),
		Status: queue.GeneratedWorkloadDeploymentExecutionStarted,
	}
	completed := started
	completed.Slot = queue.GeneratedDeploymentSlotInitialStart
	completed.Status = queue.GeneratedWorkloadDeploymentExecutionCompleted
	expected, ok := recoveredStartedForwardExecutionDetail(
		&queue.GeneratedWorkloadDeploymentEvidenceSnapshot{Executions: []queue.GeneratedWorkloadDeploymentExecutionRecord{completed, started}},
	)
	if !ok {
		t.Fatal("started forward execution did not produce a quiescence detail")
	}
	runtime := &directCodingSessionDeploymentRuntime{
		executions: map[int]queue.GeneratedWorkloadDeploymentExecutionRecord{
			started.Slot.Ordinal: started, completed.Slot.Ordinal: completed,
		},
	}
	actual, ok := runtime.startedForwardExecutionDetail()
	if !ok || actual != expected {
		t.Fatalf("runtime quiescence detail=%q want %q", actual, expected)
	}
	for file, snippets := range map[string][]string{
		"v3_coding_deployment_early_recovery.go": {
			"startedDetail, started := recoveredStartedForwardExecutionDetail(evidence)",
			"transition.DetailSHA256 = startedDetail",
		},
		"v3_coding_deployment_rollback.go": {
			"terminal.DetailSHA256 = detail",
			"startedDetail, started := runtime.startedForwardExecutionDetail()",
			"code = \"external_quiescence_unproven\"",
			"detail = startedDetail",
		},
	} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, snippet := range snippets {
			if !strings.Contains(string(source), snippet) {
				t.Fatalf("%s lacks exact quiescence binding %q", file, snippet)
			}
		}
	}
}

func directCodingAppliedRecoveryEvidenceFixture() (
	queue.GeneratedWorkloadDeploymentSnapshot,
	queue.GeneratedWorkloadDeploymentEvidenceSnapshot,
	directCodingDeploymentObservation,
) {
	operationID := "generated_workload_deployment_" + strings.Repeat("a", 64)
	verificationID := "generated_workload_verification_" + strings.Repeat("b", 64)
	workerObservation := deploymentLifecycleObservationFixture()
	queueObservation := directCodingQueueDeploymentObservation(workerObservation)
	slots := []queue.GeneratedWorkloadDeploymentLifecycleSlot{
		queue.GeneratedDeploymentSlotBuild,
		queue.GeneratedDeploymentSlotInitialStart,
		queue.GeneratedDeploymentSlotInitialObserve,
		queue.GeneratedDeploymentSlotRestart,
		queue.GeneratedDeploymentSlotRestartStart,
		queue.GeneratedDeploymentSlotFinalObserve,
	}
	commands := make([]queue.GeneratedWorkloadDeploymentExecutionCommand, len(slots))
	executions := make([]queue.GeneratedWorkloadDeploymentExecutionRecord, len(slots))
	executionIDs := make([]int64, len(slots))
	succeeded := true
	for index, slot := range slots {
		evidenceID := int64(100 + index)
		commands[index] = queue.GeneratedWorkloadDeploymentExecutionCommand{
			Slot: slot, CommandSHA256: strings.Repeat(string(rune('c'+index)), 64),
			WorkspaceSHA256: strings.Repeat("f", 64),
		}
		executions[index] = queue.GeneratedWorkloadDeploymentExecutionRecord{
			OperationID: operationID, Slot: slot,
			CommandSHA256:   commands[index].CommandSHA256,
			WorkspaceSHA256: commands[index].WorkspaceSHA256,
			Status:          queue.GeneratedWorkloadDeploymentExecutionCompleted,
			Succeeded:       &succeeded, EvidenceID: evidenceID,
		}
		executionIDs[index] = evidenceID
	}
	observations := []queue.GeneratedWorkloadDeploymentObservationRecord{
		{
			OperationID: operationID, Slot: queue.GeneratedDeploymentSlotInitialObserve,
			CommandEvidenceID: executions[2].EvidenceID,
			Observation:       queueObservation, EvidenceID: 201,
		},
		{
			OperationID: operationID, Slot: queue.GeneratedDeploymentSlotFinalObserve,
			CommandEvidenceID: executions[5].EvidenceID,
			Observation:       queueObservation, EvidenceID: 202,
		},
	}
	receipt := queue.GeneratedWorkloadDeploymentReceipt{
		OperationID: operationID, ComposeProject: queueObservation.Project,
		EndpointScheme:                 queueObservation.Endpoint.Scheme,
		EndpointHost:                   queueObservation.Endpoint.Host,
		EndpointPort:                   queueObservation.Endpoint.Port,
		EndpointPath:                   queueObservation.Endpoint.Path,
		WorkspaceVerificationReceiptID: verificationID,
		ExecutionEvidenceIDs:           executionIDs,
		ObservationEvidenceIDs:         []int64{201, 202},
	}
	for _, service := range queueObservation.Services {
		receipt.Services = append(receipt.Services,
			queue.GeneratedWorkloadDeploymentServiceReceipt{
				Service: service.Service, ContainerID: service.ContainerID,
				ImageDigest:   service.ImageDigest,
				RestartPolicy: queue.GeneratedWorkloadDeploymentRestartPolicy(service.RestartPolicy),
				State:         service.State, Health: service.Health,
			},
		)
	}
	snapshot := queue.GeneratedWorkloadDeploymentSnapshot{
		Record: queue.GeneratedWorkloadDeploymentRecord{
			OperationID: operationID, State: queue.GeneratedWorkloadDeploymentApplied,
			ReceiptSHA256: strings.Repeat("d", 64), EvidenceID: 301,
		},
		Receipt: &receipt,
	}
	evidence := queue.GeneratedWorkloadDeploymentEvidenceSnapshot{
		Verification: queue.GeneratedWorkloadVerificationRecord{ID: verificationID},
		Binding: queue.GeneratedWorkloadDeploymentVerificationBinding{
			OperationID: operationID, VerificationID: verificationID,
			LifecycleManifest: queue.GeneratedWorkloadDeploymentLifecycleManifest{Commands: commands},
		},
		Executions: executions, Observations: observations,
	}
	return snapshot, evidence, workerObservation
}
