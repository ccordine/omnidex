package queue

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
)

func lockAndValidateGeneratedDeploymentEvidenceRailTx(
	ctx context.Context,
	tx pgx.Tx,
	command GeneratedWorkloadDeploymentCommand,
	operationID string,
	executor GeneratedWorkloadDeploymentExecutor,
	receipt GeneratedWorkloadDeploymentReceipt,
) error {
	if err := validateGeneratedDeploymentEvidenceIDs(receipt.ExecutionEvidenceIDs, 6, 9); err != nil {
		return fmt.Errorf("generated deployment execution receipt: %w", err)
	}
	if err := validateGeneratedDeploymentEvidenceIDs(receipt.ObservationEvidenceIDs, 2, 2); err != nil {
		return fmt.Errorf("generated deployment observation receipt: %w", err)
	}
	binding, found, err := loadGeneratedDeploymentVerificationBindingTx(ctx, tx, operationID, true)
	if err != nil || !found {
		return fmt.Errorf("deployment workspace verification binding is invalid: %w", err)
	}
	if err := lockGeneratedDeploymentVerificationIdentitiesTx(
		ctx, tx, binding.VerificationID, receipt.WorkspaceVerificationReceiptID,
	); err != nil {
		return err
	}
	verification, found, err := loadGeneratedWorkloadVerificationByIDTx(ctx, tx, binding.VerificationID)
	if err != nil || !found {
		return fmt.Errorf("deployment workspace verification receipt is unavailable: %w", err)
	}
	executions, err := lockGeneratedDeploymentExecutionsTx(ctx, tx, operationID)
	if err != nil {
		return err
	}
	observations := make([]GeneratedWorkloadDeploymentObservationRecord, 0, 2)
	for _, slot := range []GeneratedWorkloadDeploymentLifecycleSlot{
		GeneratedDeploymentSlotInitialObserve, GeneratedDeploymentSlotFinalObserve,
	} {
		observation, found, err := loadGeneratedDeploymentObservationTx(ctx, tx, operationID, slot.Ordinal, true)
		if err != nil || !found {
			return fmt.Errorf("deployment receipt lacks exact %s observation: %w", slot.Name, err)
		}
		observations = append(observations, observation)
	}
	lockIDs := append([]int64{verification.EvidenceID}, verification.CommandEvidenceIDs...)
	lockIDs = append(lockIDs, receipt.ExecutionEvidenceIDs...)
	lockIDs = append(lockIDs, receipt.ObservationEvidenceIDs...)
	if len(lockIDs) > MaxGeneratedWorkloadDeploymentReceiptEvidence {
		return fmt.Errorf("deployment receipt exceeds the %d-row evidence bound", MaxGeneratedWorkloadDeploymentReceiptEvidence)
	}
	if err := lockGeneratedDeploymentEvidenceIDsTx(ctx, tx, lockIDs); err != nil {
		return err
	}
	if binding.VerificationID != receipt.WorkspaceVerificationReceiptID ||
		binding.WorkspaceSHA256 != command.WorkspaceSHA256 {
		return fmt.Errorf("deployment workspace verification binding differs from receipt")
	}
	manifestJSON, manifestSHA, err := canonicalGeneratedDeploymentLifecycleManifest(
		command, binding.LifecycleManifest,
	)
	if err != nil || manifestSHA != binding.LifecycleManifestSHA256 || manifestJSON == "" {
		return fmt.Errorf("deployment lifecycle manifest binding is invalid: %w", err)
	}
	if len(executions) != len(binding.LifecycleManifest.Commands) {
		return fmt.Errorf("deployment execution set differs from lifecycle manifest")
	}
	executionEvidenceIDs := make([]int64, len(executions))
	bySlot := make(map[GeneratedWorkloadDeploymentLifecycleSlot]GeneratedWorkloadDeploymentExecutionRecord, len(executions))
	for index, execution := range executions {
		expected := binding.LifecycleManifest.Commands[index]
		if execution.Status != GeneratedWorkloadDeploymentExecutionCompleted ||
			execution.Succeeded == nil || !*execution.Succeeded || execution.EvidenceID <= 0 ||
			execution.StepAttempt != executor.StepAttempt || execution.WorkerID != executor.WorkerID ||
			execution.Slot != expected.Slot || execution.CommandSHA256 != expected.CommandSHA256 ||
			execution.WorkspaceSHA256 != expected.WorkspaceSHA256 {
			return fmt.Errorf("deployment execution %d differs from successful lifecycle manifest", index)
		}
		bySlot[execution.Slot] = execution
		executionEvidenceIDs[index] = execution.EvidenceID
	}
	if err := validateGeneratedDeploymentSuccessfulSlots(bySlot); err != nil {
		return err
	}
	sort.Slice(executionEvidenceIDs, func(left, right int) bool {
		return executionEvidenceIDs[left] < executionEvidenceIDs[right]
	})
	if !equalGeneratedDeploymentEvidenceIDs(executionEvidenceIDs, receipt.ExecutionEvidenceIDs) {
		return fmt.Errorf("deployment receipt execution evidence differs from lifecycle records")
	}
	observationIDs := []int64{observations[0].EvidenceID, observations[1].EvidenceID}
	sort.Slice(observationIDs, func(left, right int) bool { return observationIDs[left] < observationIDs[right] })
	if !equalGeneratedDeploymentEvidenceIDs(observationIDs, receipt.ObservationEvidenceIDs) {
		return fmt.Errorf("deployment receipt observation evidence differs from canonical observations")
	}
	for _, observation := range observations {
		if observation.Slot != GeneratedDeploymentSlotInitialObserve &&
			observation.Slot != GeneratedDeploymentSlotFinalObserve ||
			observation.CommandEvidenceID != bySlot[observation.Slot].EvidenceID {
			return fmt.Errorf("deployment observation command binding differs")
		}
	}
	if observations[0].Observation.SHA256 != observations[1].Observation.SHA256 {
		return fmt.Errorf("deployment observation identity changed across restart")
	}
	return requireGeneratedDeploymentReceiptMatchesObservation(receipt, observations[1].Observation)
}

func lockGeneratedDeploymentVerificationIdentitiesTx(
	ctx context.Context, tx pgx.Tx, ids ...string,
) error {
	unique := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if !validSHA256ID(id, "generated_workload_verification_") {
			return fmt.Errorf("deployment cited an invalid workspace verification identity")
		}
		unique[id] = struct{}{}
	}
	ordered := make([]string, 0, len(unique))
	for id := range unique {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	rows, err := tx.Query(ctx, `
		SELECT id FROM generated_workload_verifications
		WHERE id=ANY($1::text[]) ORDER BY id FOR KEY SHARE
	`, ordered)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(ordered) {
		return fmt.Errorf("deployment workspace verification lock set is incomplete")
	}
	return nil
}

func lockGeneratedDeploymentExecutionsTx(
	ctx context.Context, tx pgx.Tx, operationID string,
) ([]GeneratedWorkloadDeploymentExecutionRecord, error) {
	rows, err := tx.Query(ctx, `
		SELECT slot_ordinal FROM generated_workload_deployment_executions
		WHERE operation_id=$1 ORDER BY slot_ordinal FOR KEY SHARE
	`, operationID)
	if err != nil {
		return nil, err
	}
	var ordinals []int
	for rows.Next() {
		var ordinal int
		if err := rows.Scan(&ordinal); err != nil {
			rows.Close()
			return nil, err
		}
		ordinals = append(ordinals, ordinal)
	}
	rows.Close()
	if len(ordinals) < 6 || len(ordinals) > 9 {
		return nil, fmt.Errorf("deployment successful lifecycle requires 6-9 exact executions")
	}
	records := make([]GeneratedWorkloadDeploymentExecutionRecord, 0, len(ordinals))
	for _, ordinal := range ordinals {
		record, found, err := loadGeneratedDeploymentExecutionTx(ctx, tx, operationID, ordinal, false)
		if err != nil || !found {
			return nil, fmt.Errorf("load locked deployment execution %d: %w", ordinal, err)
		}
		records = append(records, record)
	}
	return records, nil
}

func validateGeneratedDeploymentSuccessfulSlots(
	bySlot map[GeneratedWorkloadDeploymentLifecycleSlot]GeneratedWorkloadDeploymentExecutionRecord,
) error {
	for _, slot := range []GeneratedWorkloadDeploymentLifecycleSlot{
		GeneratedDeploymentSlotBuild, GeneratedDeploymentSlotInitialStart,
		GeneratedDeploymentSlotInitialObserve, GeneratedDeploymentSlotRestart,
		GeneratedDeploymentSlotRestartStart, GeneratedDeploymentSlotFinalObserve,
	} {
		if _, exists := bySlot[slot]; !exists {
			return fmt.Errorf("deployment successful lifecycle lacks %s", slot.Name)
		}
	}
	_, migrate := bySlot[GeneratedDeploymentSlotMigrate]
	_, write := bySlot[GeneratedDeploymentSlotStateWrite]
	_, read := bySlot[GeneratedDeploymentSlotStateRead]
	if migrate != write || migrate != read {
		return fmt.Errorf("deployment state persistence proof is incomplete")
	}
	return nil
}

func lockGeneratedDeploymentEvidenceIDsTx(
	ctx context.Context, tx pgx.Tx, ids []int64,
) error {
	unique := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return fmt.Errorf("deployment cited an invalid evidence identity")
		}
		unique[id] = struct{}{}
	}
	ordered := make([]int64, 0, len(unique))
	for id := range unique {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(left, right int) bool { return ordered[left] < ordered[right] })
	rows, err := tx.Query(ctx, `SELECT id FROM evidence WHERE id=ANY($1::bigint[]) ORDER BY id FOR KEY SHARE`, ordered)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(ordered) {
		return fmt.Errorf("deployment evidence lock set is incomplete")
	}
	return nil
}

func requireGeneratedDeploymentReceiptMatchesObservation(
	receipt GeneratedWorkloadDeploymentReceipt,
	observation GeneratedWorkloadDeploymentObservation,
) error {
	if receipt.ComposeProject != observation.Project || receipt.EndpointScheme != observation.Endpoint.Scheme ||
		receipt.EndpointHost != observation.Endpoint.Host || receipt.EndpointPort != observation.Endpoint.Port ||
		receipt.EndpointPath != observation.Endpoint.Path || len(receipt.Services) != len(observation.Services) {
		return fmt.Errorf("deployment receipt differs from final canonical observation")
	}
	for index, service := range receipt.Services {
		observed := observation.Services[index]
		if service.Service != observed.Service || service.ContainerID != observed.ContainerID ||
			service.ImageDigest != observed.ImageDigest || string(service.RestartPolicy) != observed.RestartPolicy ||
			service.State != observed.State || service.Health != observed.Health {
			return fmt.Errorf("deployment receipt service %d differs from final observation", index)
		}
	}
	return nil
}
