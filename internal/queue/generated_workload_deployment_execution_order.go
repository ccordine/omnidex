package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func requireGeneratedDeploymentExecutionAppendTx(
	ctx context.Context,
	tx pgx.Tx,
	operationID string,
	authority model.StepAttemptAuthority,
	execution GeneratedWorkloadDeploymentExecutionCommand,
) error {
	binding, found, err := loadGeneratedDeploymentVerificationBindingTx(
		ctx, tx, operationID, true,
	)
	if err != nil || !found {
		return fmt.Errorf("load deployment lifecycle append authority: %w", err)
	}
	target := -1
	for index, expected := range binding.LifecycleManifest.Commands {
		if expected.Slot == execution.Slot {
			if expected != execution {
				return fmt.Errorf("deployment execution differs from durable lifecycle manifest")
			}
			target = index
			break
		}
	}
	if target < 0 {
		return fmt.Errorf("deployment lifecycle manifest omits requested execution")
	}
	for index, expected := range binding.LifecycleManifest.Commands {
		record, exists, err := loadGeneratedDeploymentExecutionTx(
			ctx, tx, operationID, expected.Slot.Ordinal, true,
		)
		if err != nil {
			return err
		}
		if index < target {
			if !exists || record.Status != GeneratedWorkloadDeploymentExecutionCompleted ||
				record.Succeeded == nil || !*record.Succeeded ||
				record.StepAttempt != authority.Attempt || record.WorkerID != authority.WorkerID {
				return fmt.Errorf("deployment execution predecessors are not exact current successful results")
			}
			continue
		}
		if exists {
			return fmt.Errorf("deployment execution is not the first absent lifecycle command")
		}
	}
	var cleanupExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
		 SELECT 1 FROM generated_workload_deployment_rollback_attempts WHERE operation_id=$1
		 UNION ALL
		 SELECT 1 FROM generated_workload_deployment_rollback_observations WHERE operation_id=$1
		)
	`, operationID).Scan(&cleanupExists); err != nil {
		return fmt.Errorf("inspect deployment execution cleanup fence: %w", err)
	}
	if cleanupExists {
		return fmt.Errorf("deployment execution cannot append after cleanup reconciliation")
	}
	return nil
}
