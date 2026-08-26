package worker

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
)

func (recovery *directCodingSessionDeploymentRecovery) recoveredCleanupTransition(
	snapshot *queue.GeneratedWorkloadDeploymentSnapshot,
	evidence *queue.GeneratedWorkloadDeploymentEvidenceSnapshot,
) (bool, queue.GeneratedWorkloadDeploymentTransition, error) {
	if snapshot == nil || evidence == nil {
		return false, queue.GeneratedWorkloadDeploymentTransition{},
			fmt.Errorf("recovered cleanup routing requires exact deployment evidence")
	}
	attempt, err := recovery.session.runtime.svc.repo.
		CurrentGeneratedWorkloadDeploymentRollbackAttempt(
			recovery.session.runtime.ctx, snapshot.Command,
		)
	if err != nil {
		return false, queue.GeneratedWorkloadDeploymentTransition{},
			fmt.Errorf("load recovered deployment cleanup attempt: %w", err)
	}
	preObservation, err := recovery.session.runtime.svc.repo.
		CurrentGeneratedWorkloadDeploymentPreRollbackObservation(
			recovery.session.runtime.ctx, snapshot.Command,
		)
	if err != nil {
		return false, queue.GeneratedWorkloadDeploymentTransition{},
			fmt.Errorf("load recovered pre-command cleanup observation: %w", err)
	}
	material := []string{
		snapshot.Record.OperationID, string(snapshot.Record.State),
	}
	cleanup := attempt != nil || preObservation != nil
	if attempt != nil {
		material = append(material, strconv.FormatInt(attempt.StepAttempt, 10),
			attempt.CommandSHA256, attempt.Status, attempt.ResultSHA256)
	}
	if preObservation != nil {
		material = append(material, "pre_attempt",
			strconv.FormatInt(preObservation.ObserverStepAttempt, 10),
			preObservation.Outcome, preObservation.Observation.SHA256)
	}
	for _, execution := range evidence.Executions {
		cleanup = true
		material = append(material, execution.Slot.Name,
			strconv.FormatInt(execution.StepAttempt, 10), execution.CommandSHA256,
			execution.Status, execution.ResultSHA256)
	}
	if !cleanup {
		return false, queue.GeneratedWorkloadDeploymentTransition{}, nil
	}
	return true, queue.GeneratedWorkloadDeploymentTransition{
		State:        queue.GeneratedWorkloadDeploymentRolledBack,
		Code:         "recovered_side_effect",
		DetailSHA256: directCodingDigest(strings.Join(material, "\x00")),
	}, nil
}

func recoveredStartedForwardExecutionDetail(
	evidence *queue.GeneratedWorkloadDeploymentEvidenceSnapshot,
) (string, bool) {
	if evidence == nil {
		return "", false
	}
	material := make([]string, 0, len(evidence.Executions))
	for _, execution := range evidence.Executions {
		if execution.Status != queue.GeneratedWorkloadDeploymentExecutionStarted {
			continue
		}
		material = append(material, strings.Join([]string{
			execution.OperationID, execution.Slot.Name,
			strconv.Itoa(execution.Slot.Ordinal), strconv.FormatInt(execution.StepAttempt, 10),
			execution.WorkerID, execution.CommandSHA256, execution.WorkspaceSHA256,
		}, "\x00"))
	}
	if len(material) == 0 {
		return "", false
	}
	sort.Strings(material)
	return directCodingDigest(strings.Join(material, "\x01")), true
}

func recoveredDeploymentHasInitialStartExecution(
	evidence *queue.GeneratedWorkloadDeploymentEvidenceSnapshot,
) bool {
	if evidence == nil {
		return false
	}
	for _, execution := range evidence.Executions {
		if execution.Slot == queue.GeneratedDeploymentSlotInitialStart {
			return true
		}
	}
	return false
}
