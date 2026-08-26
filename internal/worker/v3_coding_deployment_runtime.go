package worker

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/gryph/omnidex/internal/operation"
	"github.com/gryph/omnidex/internal/queue"
)

type directCodingSessionDeploymentRuntime struct {
	session      *directCodingSession
	prepared     directCodingPreparedDeployment
	executions   map[int]queue.GeneratedWorkloadDeploymentExecutionRecord
	observations map[int]queue.GeneratedWorkloadDeploymentObservationRecord
}

func (runtime *directCodingSessionDeploymentRuntime) Transition(
	transition queue.GeneratedWorkloadDeploymentTransition,
) (queue.GeneratedWorkloadDeploymentRecord, error) {
	record, err := runtime.session.runtime.svc.repo.TransitionGeneratedWorkloadDeployment(
		runtime.session.runtime.ctx, runtime.session.runtime.claim.Authority,
		runtime.prepared.command, transition,
	)
	return record, err
}

func (runtime *directCodingSessionDeploymentRuntime) Execute(
	slot queue.GeneratedWorkloadDeploymentLifecycleSlot,
) error {
	_, record, _, err := runtime.execute(slot)
	if err != nil {
		return err
	}
	if record.Succeeded == nil || !*record.Succeeded {
		kind, _ := directCodingDeploymentKindForSlot(slot)
		return fmt.Errorf("deployment %s failed with persisted execution evidence %d", kind, record.EvidenceID)
	}
	runtime.rememberExecution(record)
	return nil
}

func (runtime *directCodingSessionDeploymentRuntime) execute(
	slot queue.GeneratedWorkloadDeploymentLifecycleSlot,
) (operation.Result, queue.GeneratedWorkloadDeploymentExecutionRecord, bool, error) {
	execution, kind, err := directCodingPreparedDeploymentExecution(runtime.prepared, slot)
	if err != nil {
		return operation.Result{}, queue.GeneratedWorkloadDeploymentExecutionRecord{}, false, err
	}
	executionRoot, err := runtime.prepared.snapshot.ExecutionRoot()
	if err != nil {
		return operation.Result{}, queue.GeneratedWorkloadDeploymentExecutionRecord{}, false,
			fmt.Errorf("bind deployment %s to sealed source snapshot: %w", kind, err)
	}
	if runtime.prepared.executionRoot != executionRoot {
		return operation.Result{}, queue.GeneratedWorkloadDeploymentExecutionRecord{}, false,
			fmt.Errorf("%w: deployment execution root differs from sealed snapshot", errDirectCodingDeploymentSnapshotDrift)
	}
	if err := runtime.session.qualifyDirectCodingDeploymentRuntime(
		runtime.prepared.executionRoot, runtime.prepared.command.ProfileID,
	); err != nil {
		return operation.Result{}, queue.GeneratedWorkloadDeploymentExecutionRecord{}, false,
			fmt.Errorf("qualify deployment %s runtime before durable side-effect journal: %w", kind, err)
	}
	record, created, err := beginDirectCodingDeploymentExecutionAfterNamespaceRequalification(
		func() error {
			if err := runtime.requalifyNamespaceBeforeProtectedExecution(execution); err != nil {
				cause := fmt.Errorf(
					"requalify deployment %s namespace before durable side-effect journal: %w",
					kind, err,
				)
				if errors.Is(err, errDirectCodingDeploymentNamespaceOccupied) {
					return newDirectCodingDeploymentNamespaceFailure(execution.Slot, false, cause)
				}
				return cause
			}
			return nil
		},
		func() (queue.GeneratedWorkloadDeploymentExecutionRecord, bool, error) {
			return runtime.session.runtime.svc.repo.BeginGeneratedWorkloadDeploymentExecution(
				runtime.session.runtime.ctx, runtime.session.runtime.claim.Authority,
				runtime.prepared.command, execution,
			)
		},
	)
	if err != nil {
		return operation.Result{}, queue.GeneratedWorkloadDeploymentExecutionRecord{}, false, err
	}
	runtime.rememberExecution(record)
	if !created {
		if record.Status == queue.GeneratedWorkloadDeploymentExecutionCompleted && record.Succeeded != nil {
			return operation.Result{}, record, false, nil
		}
		return operation.Result{}, record, false, fmt.Errorf(
			"deployment execution %s may have produced a side effect and requires reconciliation", slot.Name,
		)
	}
	result, err := executeDirectCodingSnapshotBoundCommand(
		runtime.prepared.snapshot, runtime.prepared.executionRoot,
		func(root string) (operation.Result, error) {
			if directCodingDeploymentProtectedNamespaceSlot(execution.Slot) {
				return runtime.session.executeProtectedDirectCodingDeploymentCommand(
					root, kind, runtime.prepared.command.ProfileID,
					runtime.prepared.project, runtime.prepared.descriptor,
					runtime.prepared.environment,
					func() error {
						if _, err := proveDirectCodingDeploymentNamespaceVacant(
							runtime.session.runtime.ctx, runtime.prepared.socketPath,
							runtime.prepared.command.ComposeProject,
						); err != nil {
							cause := fmt.Errorf(
								"reobserve protected namespace immediately before command spawn: %w", err,
							)
							if errors.Is(err, errDirectCodingDeploymentNamespaceOccupied) {
								return newDirectCodingDeploymentNamespaceFailure(execution.Slot, true, cause)
							}
							return cause
						}
						return nil
					},
				)
			}
			return runtime.session.executeDirectCodingDeploymentCommand(
				root, kind, runtime.prepared.command.ProfileID,
				runtime.prepared.project, runtime.prepared.descriptor,
				runtime.prepared.environment,
			)
		},
	)
	if err != nil {
		return operation.Result{}, record, true, err
	}
	record, err = runtime.session.runtime.svc.repo.CompleteGeneratedWorkloadDeploymentExecution(
		runtime.session.runtime.ctx, runtime.session.runtime.claim.Authority,
		runtime.prepared.command, execution, result.Evidence[0],
	)
	if err != nil {
		return operation.Result{}, record, true, err
	}
	return result, record, true, nil
}

func (runtime *directCodingSessionDeploymentRuntime) requalifyNamespaceBeforeProtectedExecution(
	execution queue.GeneratedWorkloadDeploymentExecutionCommand,
) error {
	required, err := runtime.session.runtime.svc.repo.GeneratedWorkloadDeploymentNeedsNamespaceRequalification(
		runtime.session.runtime.ctx, runtime.session.runtime.claim.Authority,
		runtime.prepared.command, execution,
	)
	if err != nil || !required {
		return err
	}
	proof, err := proveDirectCodingDeploymentNamespaceVacant(
		runtime.session.runtime.ctx, runtime.prepared.socketPath,
		runtime.prepared.command.ComposeProject,
	)
	if err != nil {
		return err
	}
	_, _, err = runtime.session.runtime.svc.repo.RecordGeneratedWorkloadDeploymentNamespaceRequalification(
		runtime.session.runtime.ctx, runtime.session.runtime.claim.Authority,
		runtime.prepared.command, execution, proof,
	)
	return err
}

func (runtime *directCodingSessionDeploymentRuntime) Observe(
	slot queue.GeneratedWorkloadDeploymentLifecycleSlot,
) (directCodingDeploymentObservation, error) {
	result, execution, created, err := runtime.execute(slot)
	if err != nil {
		return directCodingDeploymentObservation{}, err
	}
	if execution.Succeeded == nil || !*execution.Succeeded {
		return directCodingDeploymentObservation{}, fmt.Errorf("deployment observation command failed")
	}
	runtime.rememberExecution(execution)
	if !created {
		return runtime.reloadObservation(slot)
	}
	composePS, err := directCodingDeploymentComposePSOutput(result)
	if err != nil {
		return directCodingDeploymentObservation{}, err
	}
	observation, err := observeDirectCodingDeployment(
		runtime.session.runtime.ctx, runtime.prepared.socketPath,
		composePS, runtime.prepared.observationRequest,
	)
	if err != nil {
		return directCodingDeploymentObservation{}, err
	}
	queueObservation := directCodingQueueDeploymentObservation(observation)
	executionCommand, err := directCodingDeploymentManifestCommand(runtime.prepared.manifest, slot)
	if err != nil {
		return directCodingDeploymentObservation{}, err
	}
	observed, err := runtime.session.runtime.svc.repo.RecordGeneratedWorkloadDeploymentObservation(
		runtime.session.runtime.ctx, runtime.session.runtime.claim.Authority,
		runtime.prepared.command, executionCommand, queueObservation,
	)
	if err != nil {
		return directCodingDeploymentObservation{}, err
	}
	runtime.rememberObservation(observed)
	return observation, nil
}

func (runtime *directCodingSessionDeploymentRuntime) reloadObservation(
	slot queue.GeneratedWorkloadDeploymentLifecycleSlot,
) (directCodingDeploymentObservation, error) {
	snapshot, err := runtime.session.runtime.svc.repo.GeneratedWorkloadDeploymentEvidence(
		runtime.session.runtime.ctx, runtime.prepared.command.Authority.JobID,
		runtime.prepared.command.Authority.Generation,
	)
	if err != nil || snapshot == nil {
		return directCodingDeploymentObservation{}, fmt.Errorf("reload deployment observation %s: %w", slot.Name, err)
	}
	for _, observation := range snapshot.Observations {
		if observation.Slot == slot {
			runtime.rememberObservation(observation)
			return directCodingWorkerDeploymentObservation(observation.Observation), nil
		}
	}
	return directCodingDeploymentObservation{}, fmt.Errorf("completed deployment observation %s has no canonical evidence", slot.Name)
}

func (runtime *directCodingSessionDeploymentRuntime) EvidenceIDs() ([]int64, []int64, error) {
	executionIDs := make([]int64, 0, len(runtime.prepared.manifest.Commands))
	for _, command := range runtime.prepared.manifest.Commands {
		record, exists := runtime.executions[command.Slot.Ordinal]
		if !exists || record.EvidenceID <= 0 {
			return nil, nil, fmt.Errorf("deployment evidence lacks execution slot %s", command.Slot.Name)
		}
		executionIDs = append(executionIDs, record.EvidenceID)
	}
	observationIDs := make([]int64, 0, 2)
	for _, slot := range []queue.GeneratedWorkloadDeploymentLifecycleSlot{
		queue.GeneratedDeploymentSlotInitialObserve, queue.GeneratedDeploymentSlotFinalObserve,
	} {
		record, exists := runtime.observations[slot.Ordinal]
		if !exists || record.EvidenceID <= 0 {
			return nil, nil, fmt.Errorf("deployment evidence lacks observation slot %s", slot.Name)
		}
		observationIDs = append(observationIDs, record.EvidenceID)
	}
	sort.Slice(executionIDs, func(left, right int) bool { return executionIDs[left] < executionIDs[right] })
	sort.Slice(observationIDs, func(left, right int) bool { return observationIDs[left] < observationIDs[right] })
	return executionIDs, observationIDs, nil
}

func (runtime *directCodingSessionDeploymentRuntime) Seal(
	receipt queue.GeneratedWorkloadDeploymentReceipt,
) (queue.GeneratedWorkloadDeploymentRecord, error) {
	return runtime.session.runtime.svc.repo.SealGeneratedWorkloadDeploymentApplied(
		runtime.session.runtime.ctx, runtime.session.runtime.claim.Authority,
		runtime.prepared.command, receipt,
	)
}

func (runtime *directCodingSessionDeploymentRuntime) CompleteDeployment(operationID, receiptSHA256 string) error {
	_, err := runtime.session.cognition.CompleteDeployment(operationID, receiptSHA256)
	return err
}

func (*directCodingSessionDeploymentRuntime) Now() time.Time {
	return directCodingDeploymentTimestamp()
}

func (runtime *directCodingSessionDeploymentRuntime) rememberExecution(
	record queue.GeneratedWorkloadDeploymentExecutionRecord,
) {
	if runtime.executions == nil {
		runtime.executions = make(map[int]queue.GeneratedWorkloadDeploymentExecutionRecord)
	}
	runtime.executions[record.Slot.Ordinal] = record
}

func (runtime *directCodingSessionDeploymentRuntime) startedForwardExecutionDetail() (string, bool) {
	evidence := &queue.GeneratedWorkloadDeploymentEvidenceSnapshot{
		Executions: make([]queue.GeneratedWorkloadDeploymentExecutionRecord, 0, len(runtime.executions)),
	}
	for _, execution := range runtime.executions {
		evidence.Executions = append(evidence.Executions, execution)
	}
	return recoveredStartedForwardExecutionDetail(evidence)
}

func (runtime *directCodingSessionDeploymentRuntime) rememberObservation(
	record queue.GeneratedWorkloadDeploymentObservationRecord,
) {
	if runtime.observations == nil {
		runtime.observations = make(map[int]queue.GeneratedWorkloadDeploymentObservationRecord)
	}
	runtime.observations[record.Slot.Ordinal] = record
}

func directCodingPreparedDeploymentExecution(
	prepared directCodingPreparedDeployment,
	slot queue.GeneratedWorkloadDeploymentLifecycleSlot,
) (queue.GeneratedWorkloadDeploymentExecutionCommand, directCodingDeploymentCommandKind, error) {
	kind, err := directCodingDeploymentKindForSlot(slot)
	if err != nil {
		return queue.GeneratedWorkloadDeploymentExecutionCommand{}, "", err
	}
	execution, err := directCodingDeploymentManifestCommand(prepared.manifest, slot)
	return execution, kind, err
}

func directCodingDeploymentComposePSOutput(result operation.Result) ([]byte, error) {
	if truncated, ok := result.Output["stdout_truncated"].(bool); !ok || truncated {
		return nil, fmt.Errorf("deployment Compose observation output is missing or truncated")
	}
	stdout, ok := result.Output["stdout"].(string)
	if !ok || stdout == "" {
		return nil, fmt.Errorf("deployment Compose observation output is empty")
	}
	return []byte(stdout), nil
}
