package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ReconcileCognitionRuntimeDecision(
	ctx context.Context,
	command cognitionruntime.ReconciliationCommand,
) (cognitionruntime.ReconciliationReceipt, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return cognitionruntime.ReconciliationReceipt{}, fmt.Errorf("cognition reconciliation requires PostgreSQL and context")
	}
	if err := command.Validate(); err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	authority, err := cognitionRuntimeAuthority(command.Binding)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	defer tx.Rollback(ctx)
	if _, status, _, err := requireActiveStepAttemptTx(ctx, tx, authority); err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	} else if status != model.StepStatusRunning {
		return cognitionruntime.ReconciliationReceipt{}, staleStepAttemptError(authority, "cognition reconciler is not running", nil)
	}
	header, episode, graph, err := loadCognitionSnapshotAuthorityTx(ctx, tx, CognitionRuntimeSnapshotCommand{
		Authority: authority, EpisodeID: command.Binding.Episode.ID,
	})
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	var prepared CognitionRuntimeSnapshotRecord
	if command.Recovery == nil {
		prepared, err = loadCognitionPreparedSnapshotBySHATx(
			ctx, tx, authority, episode, graph, command.SnapshotSHA256,
		)
	} else {
		prepared, _, err = preparedSnapshotForRecoveryTx(
			ctx, tx, authority, episode, graph, command.Binding, command.Recovery,
		)
	}
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	if prepared.Prepared.Snapshot.SHA256() != command.SnapshotSHA256 {
		return cognitionruntime.ReconciliationReceipt{}, fmt.Errorf(
			"%w: reconciliation snapshot differs from accepted recovery", ErrCognitionConflict,
		)
	}
	if prepared.Prepared.Snapshot.ContextProjection() != command.Projection {
		return cognitionruntime.ReconciliationReceipt{}, fmt.Errorf("%w: reconciliation projection differs from prepared snapshot", ErrCognitionConflict)
	}
	materialization, err := planCognitionObligationMaterialization(
		episode, graph, prepared.Prepared, command.Decision, command.ActionSchema,
	)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	planRevision, err := planCognitionPlanRevision(
		episode, graph, prepared.Prepared, command.Decision, command.ActionSchema,
	)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	if materialization != nil && planRevision != nil {
		return cognitionruntime.ReconciliationReceipt{}, fmt.Errorf(
			"%w: one decision cannot carry two graph materializations", ErrCognitionConflict,
		)
	}
	if receipt, found, err := loadCognitionReconciliationReplayTx(ctx, tx, command); err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	} else if found {
		if err := requireCognitionMaterializationReplayTx(
			ctx, tx, receipt.ID, materialization,
		); err != nil {
			return cognitionruntime.ReconciliationReceipt{}, err
		}
		if err := requireCognitionPlanRevisionReplayTx(
			ctx, tx, receipt.ID, planRevision,
		); err != nil {
			return cognitionruntime.ReconciliationReceipt{}, err
		}
		if err := requireCognitionBeliefRevisionReplayTx(
			ctx, tx, receipt.ID, command,
		); err != nil {
			return cognitionruntime.ReconciliationReceipt{}, err
		}
		if err := requireCognitionAttentionOutcomeReplayTx(
			ctx, tx, receipt.ID, command.Decision.Attention,
		); err != nil {
			return cognitionruntime.ReconciliationReceipt{}, err
		}
		if err := requireCognitionDecisionAcceptanceReplayTx(ctx, tx, receipt.ID, command); err != nil {
			return cognitionruntime.ReconciliationReceipt{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return cognitionruntime.ReconciliationReceipt{}, err
		}
		return receipt, nil
	}
	policyCallID, err := loadExactCognitionPolicyCallTx(ctx, tx, authority, command)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	ledger, err := restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	acceptedLedger, decisionAcceptance, err := persistCognitionSelectedDecisionTx(
		ctx, tx, authority, episode, policyCallID, prepared.Prepared, command, ledger.MaterializedState(),
	)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	proposalInput := cognitionstate.ModelProposalInput{
		Ledger: acceptedLedger, ScopeNodeID: taskstate.NodeID(command.Decision.ObligationID),
		Snapshot: prepared.Prepared.Snapshot, Decision: command.Decision, ActionSchema: command.ActionSchema,
	}
	revision, err := planCognitionBeliefRevision(
		proposalInput.Ledger, prepared.Prepared, command,
	)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	if revision != nil {
		header, proposalInput.Ledger, err = applyCognitionBeliefRevisionTx(
			ctx, tx, authority, proposalInput.Ledger, *revision,
		)
		if err != nil {
			return cognitionruntime.ReconciliationReceipt{}, err
		}
	}
	mutations, err := cognitionstate.MapModelProposals(proposalInput)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	for _, mutation := range mutations {
		if _, err := applyQueueOwnedTaskCommandTx(
			ctx, tx, authority.JobID, authority.Generation, mutation.Command(),
		); err != nil {
			return cognitionruntime.ReconciliationReceipt{}, fmt.Errorf("persist cognition model proposal: %w", err)
		}
	}
	materializationCandidate, err := obligationMaterializationCandidate(mutations, materialization)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	planRevisionCandidateID, err := planRevisionCandidate(mutations, planRevision)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	header, err = loadTaskLedgerHeaderTx(ctx, tx, authority.JobID, false)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	ledger, err = restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	factSources, err := loadCognitionFactProjectionSourcesTx(ctx, tx, episode, graph.Graph)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	setSnapshot, err := loadWorkingSetSnapshotTx(ctx, tx, header, authority.Generation, true)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	evidence, err := loadCognitionEvidenceMaterialsTx(
		ctx, tx, prepared.Prepared.CompletionEvidenceRefs,
	)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	retained, err := retainedCognitionAttention(
		setSnapshot, evidence, prepared.Prepared.Snapshot.CurrentObligation().ID,
	)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	required, err := retainedCognitionAttentionNotOverridden(retained, command.Decision.Attention)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	state, err := cognitionReconciliationState(prepared.Prepared)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	plan, err := fitCognitionReconciliationPlan(cognitionReconciliationFitInput{
		Episode: episode, Current: prepared.Prepared.Snapshot.CurrentObligation(),
		Authority: authority, Prepared: prepared.Prepared, State: state, Graph: graph.Graph,
		Ledger: ledger.MaterializedState(), Set: setSnapshot, Evidence: evidence,
		FactSources: factSources,
		Required:    required, Requested: command.Decision.Attention,
	})
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	set, _, err := applyCognitionAttentionPlanTx(ctx, tx, authority, plan)
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	receipt, err := cognitionruntime.NewReconciliationReceipt(command, header.Version, set.Version())
	if err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	if err := insertCognitionReconciliationTx(ctx, tx, authority, episode.EpisodeID, policyCallID, command, receipt); err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	if err := insertCognitionAttentionOutcomesTx(ctx, tx, receipt.ID, plan.AdvisoryOutcomes()); err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	if err := insertCognitionDecisionAcceptanceTx(ctx, tx, episode, receipt.ID, decisionAcceptance); err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	if materialization != nil {
		if err := insertCognitionObligationMaterializationTx(
			ctx, tx, episode, prepared.Prepared.GraphVersion, receipt.ID,
			episode.LedgerID, materializationCandidate, *materialization,
		); err != nil {
			return cognitionruntime.ReconciliationReceipt{}, err
		}
	}
	if planRevision != nil {
		if err := insertCognitionPlanRevisionTx(
			ctx, tx, episode, prepared.Prepared.GraphVersion, receipt.ID,
			episode.LedgerID, planRevisionCandidateID, *planRevision,
		); err != nil {
			return cognitionruntime.ReconciliationReceipt{}, err
		}
	}
	if revision != nil {
		if err := insertCognitionBeliefRevisionTx(
			ctx, tx, episode, receipt.ID, *revision,
		); err != nil {
			return cognitionruntime.ReconciliationReceipt{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return cognitionruntime.ReconciliationReceipt{}, err
	}
	return receipt, nil
}

func cognitionReconciliationState(
	prepared cognitionruntime.PreparedSnapshot,
) (cognitionstate.ProjectionState, error) {
	snapshot := prepared.Snapshot
	return cognitionstate.NewProjectionState(
		snapshot.Goal(), snapshot.CurrentRevision(), snapshot.CurrentObligation(),
		snapshot.ActionCatalog(), snapshot.Attempt(), snapshot.Budget(),
		prepared.CompletionEvidenceRefs,
	)
}
