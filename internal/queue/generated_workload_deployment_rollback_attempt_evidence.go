package queue

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/evidence"
)

func generatedDeploymentRollbackAttemptEvidence(
	command GeneratedWorkloadDeploymentCommand,
	operationID string,
	stepAttempt int64,
	plan GeneratedWorkloadDeploymentRollbackPlan,
	record evidence.Record,
) (bool, string, string, error) {
	if record.JobID != 0 && record.JobID != command.Authority.JobID ||
		record.StepID != 0 && record.StepID != command.Authority.StepID {
		return false, "", "", fmt.Errorf("deployment rollback evidence owner differs")
	}
	record.JobID = command.Authority.JobID
	record.StepID = command.Authority.StepID
	if err := record.Validate(); err != nil {
		return false, "", "", err
	}
	if record.Kind != evidence.KindCommandOutput && record.Kind != evidence.KindTestResult ||
		record.SourceType != "command" ||
		generatedDeploymentSHA(record.Command) != plan.Execution.CommandSHA256 {
		return false, "", "", fmt.Errorf("deployment rollback evidence differs from exact command digest")
	}
	succeeded, ok := record.Metadata["succeeded"].(bool)
	if !ok || record.Metadata["execution"] != true ||
		record.Metadata["side_effect_possible"] != true {
		return false, "", "", fmt.Errorf("deployment rollback evidence lacks exact side-effecting execution result")
	}
	metadata := make(map[string]any, len(record.Metadata)+7)
	for key, value := range record.Metadata {
		if key != "workspace" {
			metadata[key] = value
		}
	}
	metadata["deployment_operation_id"] = operationID
	metadata["step_attempt"] = stepAttempt
	metadata["slot"] = plan.Execution.Slot.Name
	metadata["ordinal"] = plan.Execution.Slot.Ordinal
	metadata["command_sha256"] = plan.Execution.CommandSHA256
	metadata["workspace_sha256"] = plan.Execution.WorkspaceSHA256
	metadata["postcondition_sha256"] = plan.PostconditionSHA256
	record.Metadata = metadata
	record.SourceType = generatedWorkloadDeploymentRollbackEvidenceSource
	record.SourceRef = operationID
	payload, err := json.Marshal(record)
	if err != nil {
		return false, "", "", fmt.Errorf("encode deployment rollback evidence: %w", err)
	}
	if len(payload) > 1<<20 {
		return false, "", "", fmt.Errorf("deployment rollback evidence exceeds canonical bound")
	}
	return succeeded, string(payload), generatedDeploymentSHA(string(payload)), nil
}
