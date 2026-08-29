package worker

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/gryph/omnidex/internal/queue"
)

func directCodingRecoveredFinalObservation(
	snapshot queue.GeneratedWorkloadDeploymentSnapshot,
	evidence *queue.GeneratedWorkloadDeploymentEvidenceSnapshot,
) (directCodingDeploymentObservation, error) {
	if snapshot.Record.State != queue.GeneratedWorkloadDeploymentApplied ||
		snapshot.Receipt == nil || evidence == nil {
		return directCodingDeploymentObservation{}, fmt.Errorf(
			"applied recovery requires a sealed receipt and evidence snapshot",
		)
	}
	receipt := snapshot.Receipt
	if receipt.OperationID != snapshot.Record.OperationID ||
		snapshot.Record.ReceiptSHA256 == "" || snapshot.Record.EvidenceID <= 0 ||
		receipt.WorkspaceVerificationReceiptID != evidence.Binding.VerificationID ||
		evidence.Verification.ID != evidence.Binding.VerificationID ||
		evidence.Binding.OperationID != snapshot.Record.OperationID {
		return directCodingDeploymentObservation{}, fmt.Errorf(
			"applied recovery receipt binding differs from durable evidence",
		)
	}
	if len(evidence.Executions) != len(evidence.Binding.LifecycleManifest.Commands) {
		return directCodingDeploymentObservation{}, fmt.Errorf(
			"applied recovery execution set differs from lifecycle manifest",
		)
	}
	executionIDs := make([]int64, len(evidence.Executions))
	executions := make(map[int]queue.GeneratedWorkloadDeploymentExecutionRecord, len(evidence.Executions))
	for index, execution := range evidence.Executions {
		expected := evidence.Binding.LifecycleManifest.Commands[index]
		if execution.Status != queue.GeneratedWorkloadDeploymentExecutionCompleted ||
			execution.Succeeded == nil || !*execution.Succeeded ||
			execution.EvidenceID <= 0 || execution.Slot != expected.Slot ||
			execution.CommandSHA256 != expected.CommandSHA256 ||
			execution.WorkspaceSHA256 != expected.WorkspaceSHA256 {
			return directCodingDeploymentObservation{}, fmt.Errorf(
				"applied recovery execution %d differs from successful manifest", index,
			)
		}
		executionIDs[index] = execution.EvidenceID
		executions[execution.Slot.Ordinal] = execution
	}
	sort.Slice(executionIDs, func(left, right int) bool {
		return executionIDs[left] < executionIDs[right]
	})
	if !reflect.DeepEqual(executionIDs, receipt.ExecutionEvidenceIDs) {
		return directCodingDeploymentObservation{}, fmt.Errorf(
			"applied recovery execution evidence differs from sealed receipt",
		)
	}
	observations := make(map[int]queue.GeneratedWorkloadDeploymentObservationRecord, 2)
	observationIDs := make([]int64, 0, len(evidence.Observations))
	for _, observation := range evidence.Observations {
		execution, exists := executions[observation.Slot.Ordinal]
		if !exists || observation.OperationID != snapshot.Record.OperationID ||
			observation.CommandEvidenceID != execution.EvidenceID ||
			observation.EvidenceID <= 0 {
			return directCodingDeploymentObservation{}, fmt.Errorf(
				"applied recovery observation has invalid execution binding",
			)
		}
		observations[observation.Slot.Ordinal] = observation
		observationIDs = append(observationIDs, observation.EvidenceID)
	}
	initial, hasInitial := observations[queue.GeneratedDeploymentSlotInitialObserve.Ordinal]
	final, hasFinal := observations[queue.GeneratedDeploymentSlotFinalObserve.Ordinal]
	if len(observations) != 2 || !hasInitial || !hasFinal ||
		initial.Observation.SHA256 != final.Observation.SHA256 {
		return directCodingDeploymentObservation{}, fmt.Errorf(
			"applied recovery lacks stable initial and final observations",
		)
	}
	sort.Slice(observationIDs, func(left, right int) bool {
		return observationIDs[left] < observationIDs[right]
	})
	if !reflect.DeepEqual(observationIDs, receipt.ObservationEvidenceIDs) ||
		!directCodingRecoveryReceiptMatchesObservation(*receipt, final.Observation) {
		return directCodingDeploymentObservation{}, fmt.Errorf(
			"sealed deployment receipt differs from final observation evidence",
		)
	}
	return directCodingWorkerDeploymentObservation(final.Observation), nil
}

func directCodingRecoveryReceiptMatchesObservation(
	receipt queue.GeneratedWorkloadDeploymentReceipt,
	observation queue.GeneratedWorkloadDeploymentObservation,
) bool {
	if receipt.ComposeProject != observation.Project ||
		receipt.EndpointScheme != observation.Endpoint.Scheme ||
		receipt.EndpointHost != observation.Endpoint.Host ||
		receipt.EndpointPort != observation.Endpoint.Port ||
		receipt.EndpointPath != observation.Endpoint.Path ||
		len(receipt.Services) != len(observation.Services) {
		return false
	}
	for index, service := range receipt.Services {
		observed := observation.Services[index]
		if service.Service != observed.Service ||
			service.ContainerID != observed.ContainerID ||
			service.ImageDigest != observed.ImageDigest ||
			string(service.RestartPolicy) != observed.RestartPolicy ||
			service.State != observed.State || service.Health != observed.Health {
			return false
		}
	}
	return true
}
