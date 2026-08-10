package queue

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// PrepareCognitionAction is the sole action-preparation boundary. It accepts
// only a runtime command carrying the exact durable reconciliation receipt.
func (r *Repository) PrepareCognitionAction(
	ctx context.Context,
	command cognitionruntime.PrepareActionCommand,
) (CognitionActionRecord, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return CognitionActionRecord{}, fmt.Errorf("cognition action preparation requires PostgreSQL and context")
	}
	if err := command.Binding.Validate(); err != nil {
		return CognitionActionRecord{}, err
	}
	authority, err := cognitionRuntimeAuthority(command.Binding)
	if err != nil {
		return CognitionActionRecord{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CognitionActionRecord{}, err
	}
	defer tx.Rollback(ctx)
	if _, status, _, err := requireActiveStepAttemptTx(ctx, tx, authority); err != nil {
		return CognitionActionRecord{}, err
	} else if status != model.StepStatusRunning {
		return CognitionActionRecord{}, staleStepAttemptError(authority, "cognition action step is not running", nil)
	}
	header, episode, graph, err := loadCognitionSnapshotAuthorityTx(ctx, tx, CognitionRuntimeSnapshotCommand{
		Authority: authority, EpisodeID: command.Binding.Episode.ID,
	})
	if err != nil {
		return CognitionActionRecord{}, err
	}
	var prepared CognitionRuntimeSnapshotRecord
	var recovery *cognitionruntime.AcceptedDecisionRecovery
	if command.Recovery == nil {
		prepared, err = loadCognitionPreparedSnapshotBySHATx(
			ctx, tx, authority, episode, graph, command.Coordinator.SnapshotSHA256,
		)
	} else {
		var recovered cognitionruntime.AcceptedDecisionRecovery
		prepared, recovered, err = preparedSnapshotForRecoveryTx(
			ctx, tx, authority, episode, graph, command.Binding, command.Recovery,
		)
		recovery = &recovered
	}
	if err != nil {
		return CognitionActionRecord{}, err
	}
	if prepared.Prepared.Snapshot.SHA256() != command.Coordinator.SnapshotSHA256 {
		return CognitionActionRecord{}, fmt.Errorf(
			"%w: action snapshot differs from accepted recovery", ErrCognitionConflict,
		)
	}
	reconciliation, err := loadExactCognitionReconciliationTx(ctx, tx, command)
	if err != nil {
		return CognitionActionRecord{}, err
	}
	materialization, err := planCognitionObligationMaterialization(
		episode, graph, prepared.Prepared, reconciliation.Command.Decision,
		reconciliation.Command.ActionSchema,
	)
	if err != nil {
		return CognitionActionRecord{}, err
	}
	if err := requireCognitionMaterializationReplayTx(
		ctx, tx, command.Reconciliation.ID, materialization,
	); err != nil {
		return CognitionActionRecord{}, err
	}
	set, err := loadWorkingSetSnapshotTx(ctx, tx, header, authority.Generation, true)
	if err != nil {
		return CognitionActionRecord{}, err
	}
	if command.Reconciliation.LedgerVersion != header.Version ||
		command.Reconciliation.WorkingSetVersion != set.Version {
		return CognitionActionRecord{}, fmt.Errorf(
			"%w: reconciliation versions are not the current durable state", ErrCognitionConflict,
		)
	}
	call, found, err := loadCognitionPolicyCallTx(ctx, tx, reconciliation.PolicyCallID, false)
	if err != nil || !found {
		return CognitionActionRecord{}, fmt.Errorf("%w: accepted policy call is unavailable: %v", ErrCognitionConflict, err)
	}
	if err := validateCognitionPreparedCommand(command, prepared.Prepared, call, recovery); err != nil {
		return CognitionActionRecord{}, err
	}
	decision := command.Coordinator.Decision.Clone()
	schema, exists := episode.ActionCatalog.Schema(decision.Action.Kind)
	if !exists || schema.Ref() != command.Coordinator.ActionSchema {
		return CognitionActionRecord{}, fmt.Errorf("%w: action schema is not registered", ErrCognitionConflict)
	}
	if err := requireCognitionEvidenceRefsTx(
		ctx, tx, episode.EpisodeID, episode.CurrentRevision, decision.EvidenceRefs,
	); err != nil {
		return CognitionActionRecord{}, err
	}
	actionID := cognitionActionID(episode.EpisodeID, episode.CurrentRevision, reconciliation.PolicyCallID)
	action, err := cognition.NewRegisteredAction(
		actionID, command.Binding.Attempt, schema, decision.Action, decision.EvidenceRefs,
	)
	if err != nil {
		return CognitionActionRecord{}, err
	}
	if existing, found, err := loadCognitionActionTx(ctx, tx, actionID, false); err != nil {
		return CognitionActionRecord{}, err
	} else if found {
		if err := requireExactPreparedActionReplay(existing, action, command, reconciliation.PolicyCallID); err != nil {
			return CognitionActionRecord{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CognitionActionRecord{}, err
		}
		return existing, nil
	}
	decisionJSON, decisionSHA, err := cognitionJSON(decision)
	if err != nil {
		return CognitionActionRecord{}, err
	}
	actionJSON, actionSHA, err := cognitionJSON(action)
	if err != nil {
		return CognitionActionRecord{}, err
	}
	projection := prepared.Prepared.Snapshot.ContextProjection()
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_actions (
			action_id,episode_id,job_id,generation,step_id,origin_attempt,origin_worker_id,
			obligation_node_id,policy_call_id,expected_revision,expected_revision_sha256,
			snapshot_sha256,projection_id,action_schema_id,action_schema_version,
			action_schema_sha256,decision_json,decision_sha256,registered_action_json,
			registered_action_sha256,reconciliation_id,reconciliation_sha256,status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,'prepared')
	`, action.ID, episode.EpisodeID, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID, decision.ObligationID, reconciliation.PolicyCallID,
		int64(episode.CurrentRevision.Number), episode.CurrentRevision.SHA256,
		prepared.Prepared.Snapshot.SHA256(), projection.ID, schema.ID, schema.Version,
		schema.SHA256, string(decisionJSON), decisionSHA, string(actionJSON), actionSHA,
		command.Reconciliation.ID, command.Reconciliation.SHA256)
	if err != nil {
		return CognitionActionRecord{}, fmt.Errorf("insert prepared cognition action: %w", err)
	}
	if err := insertCognitionActionEventTx(ctx, tx, action.ID, authority, CognitionActionPrepared, nil); err != nil {
		return CognitionActionRecord{}, err
	}
	record, found, err := loadCognitionActionTx(ctx, tx, action.ID, false)
	if err != nil || !found {
		return CognitionActionRecord{}, fmt.Errorf("reload prepared cognition action: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CognitionActionRecord{}, err
	}
	return record, nil
}

func requireExactPreparedActionReplay(
	existing CognitionActionRecord,
	action cognition.RegisteredAction,
	command cognitionruntime.PrepareActionCommand,
	policyCallID string,
) error {
	if existing.PolicyCallID != policyCallID || existing.ReconciliationID != command.Reconciliation.ID ||
		existing.ReconciliationSHA256 != command.Reconciliation.SHA256 ||
		!reflect.DeepEqual(existing.Action, action) || command.Coordinator.Decision == nil ||
		!reflect.DeepEqual(existing.Decision, *command.Coordinator.Decision) {
		return fmt.Errorf("%w: cognition action replay changed content", ErrCognitionConflict)
	}
	return nil
}

func requireCognitionEvidenceRefsTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
	current cognition.WorldRevision,
	refs []cognition.EvidenceRef,
) error {
	for index, ref := range refs {
		if ref.Revision.EpisodeID != episodeID || ref.Revision.Number > current.Number {
			return fmt.Errorf("cognition evidence %d is outside the current episode revision", index)
		}
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM cognition_transition_observations observations
				JOIN cognition_transitions transitions ON transitions.transition_id=observations.transition_id
				WHERE transitions.episode_id=$1 AND transitions.revision=$2
				  AND observations.observation_id=$3 AND observations.content_sha256=$4
			)
		`, episodeID, int64(ref.Revision.Number), ref.ObservationID, ref.SHA256).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("cognition evidence %d has no immutable observation", index)
		}
	}
	return nil
}
