package queue

import (
	"context"
	"fmt"
	"math"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) PrepareCognitionRuntimeSnapshot(
	ctx context.Context,
	command CognitionRuntimeSnapshotCommand,
) (CognitionRuntimeSnapshotRecord, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return CognitionRuntimeSnapshotRecord{}, fmt.Errorf("cognition snapshot preparation requires PostgreSQL and context")
	}
	if err := validateStepAttemptAuthority(command.Authority); err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	if err := cognitionEpisodeIdentityValid(command.EpisodeID); err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	defer tx.Rollback(ctx)
	if _, status, _, err := requireActiveStepAttemptTx(ctx, tx, command.Authority); err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	} else if status != model.StepStatusRunning {
		return CognitionRuntimeSnapshotRecord{}, staleStepAttemptError(command.Authority, "cognition snapshot actor is not running", nil)
	}
	header, episode, graph, err := loadCognitionSnapshotAuthorityTx(ctx, tx, command)
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	callOrdinal, budget, err := remainingCognitionBudgetTx(ctx, tx, episode)
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	current, err := oneActiveCognitionObligation(graph.Graph)
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	if replay, found, err := loadCognitionSnapshotReplayTx(
		ctx, tx, command.Authority, episode, graph, current.ID, callOrdinal, budget,
	); err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	} else if found {
		if err := tx.Commit(ctx); err != nil {
			return CognitionRuntimeSnapshotRecord{}, err
		}
		return replay, nil
	}
	ledger, err := restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	setSnapshot, err := loadWorkingSetSnapshotTx(ctx, tx, header, command.Authority.Generation, true)
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	evidence, refs, factSources, err := loadCognitionEvidencePacketTx(
		ctx, tx, episode, graph.Graph, setSnapshot, budget.MaxEvidenceRefs,
	)
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	state, err := cognitionstate.NewProjectionState(
		episode.Goal, episode.CurrentRevision, current, episode.ActionCatalog,
		cognitionAttempt(command.Authority), budget, refs,
	)
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	attention, err := retainedCognitionAttention(setSnapshot, evidence, current.ID)
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	plan, err := cognitionstate.BuildDefaultReconciliation(cognitionstate.ReconciliationInput{
		State: state, ObligationGraph: graph.Graph, Ledger: ledger.MaterializedState(),
		WorkingSet: setSnapshot, Evidence: evidence, RequiredAttention: attention,
	})
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	set, projection, err := applyCognitionAttentionPlanTx(ctx, tx, command.Authority, plan)
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	fit, err := fitCognitionPolicyProjection(cognitionProjectionFitInput{
		Episode: episode, Current: current, Attempt: cognitionAttempt(command.Authority),
		Budget: budget, CompletionEvidence: refs, Set: set, WorkID: plan.Descriptor().SourceSHA256,
		FactSources: factSources, Spec: plan.ContextSpec(), Materials: plan.Materials(), Initial: projection,
	})
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	record, err := storeLiveCognitionProjectionTx(ctx, tx, command.Authority, fit.Projection)
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	if cognitionProjectionReference(record.Projection) != fit.Snapshot.ContextProjection() {
		return CognitionRuntimeSnapshotRecord{}, fmt.Errorf(
			"%w: stored live projection differs from the measured policy envelope", ErrCognitionConflict,
		)
	}
	terminal, outcome, err := loadCognitionEnvironmentStateTx(ctx, tx, episode)
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	prepared := cognitionruntime.PreparedSnapshot{
		Snapshot: fit.Snapshot, ObligationGraph: graph.Graph.Clone(), GraphVersion: graph.Version,
		CompletionEvidenceRefs: append([]cognition.EvidenceRef{}, refs...),
		EnvironmentTerminal:    terminal, PublicOutcome: outcome,
	}
	if err := insertCognitionSnapshotJournalTx(ctx, tx, command.Authority, prepared, callOrdinal); err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CognitionRuntimeSnapshotRecord{}, err
	}
	return CognitionRuntimeSnapshotRecord{Prepared: prepared, CallOrdinal: callOrdinal}, nil
}

func remainingCognitionBudgetTx(ctx context.Context, tx pgx.Tx, episode CognitionEpisode) (uint64, cognition.RuntimeBudget, error) {
	var calls int64
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1`, episode.EpisodeID).Scan(&calls); err != nil {
		return 0, cognition.RuntimeBudget{}, fmt.Errorf("count cognition policy evidence: %w", err)
	}
	if calls < 0 || uint64(calls) > uint64(episode.Budget.RemainingPolicyCalls) || calls == math.MaxInt64 {
		return 0, cognition.RuntimeBudget{}, ErrCognitionBudgetExhausted
	}
	budget := episode.Budget
	budget.RemainingPolicyCalls -= uint32(calls)
	return uint64(calls) + 1, budget, nil
}

func cognitionAttempt(authority model.StepAttemptAuthority) cognition.AttemptRef {
	return cognition.AttemptRef{JobID: authority.JobID, Generation: authority.Generation, StepID: authority.StepID,
		Attempt: uint64(authority.Attempt), WorkerID: authority.WorkerID}
}

func oneActiveCognitionObligation(graph cognition.ObligationGraphSnapshot) (cognition.Obligation, error) {
	var current cognition.Obligation
	count := 0
	for _, obligation := range graph.Obligations {
		if obligation.Status == cognition.ObligationActive {
			current, count = obligation.Clone(), count+1
		}
	}
	if count != 1 {
		return cognition.Obligation{}, fmt.Errorf("%w: cognition graph requires exactly one active obligation, found %d", ErrCognitionConflict, count)
	}
	return current, nil
}

func applyCognitionAttentionPlanTx(
	ctx context.Context, tx pgx.Tx, authority model.StepAttemptAuthority, plan cognitionstate.ReconciliationPlan,
) (*workingset.Set, contextbuilder.Projection, error) {
	for _, mutation := range plan.Commands() {
		command := mutation.Command()
		descriptor := mutation.Descriptor()
		if command == nil {
			return nil, contextbuilder.Projection{}, fmt.Errorf("cognition attention plan contains an unavailable command")
		}
		if _, err := applyWorkingSetCommandTx(ctx, tx, authority, command, descriptor); err != nil {
			return nil, contextbuilder.Projection{}, err
		}
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, authority.JobID, false)
	if err != nil {
		return nil, contextbuilder.Projection{}, err
	}
	snapshot, err := loadWorkingSetSnapshotTx(ctx, tx, header, authority.Generation, false)
	if err != nil {
		return nil, contextbuilder.Projection{}, err
	}
	set, err := workingset.Restore(snapshot)
	if err != nil {
		return nil, contextbuilder.Projection{}, err
	}
	projection, err := contextbuilder.Build(contextbuilder.BuildInput{
		WorkID: plan.Descriptor().SourceSHA256, Spec: plan.ContextSpec(), WorkingSet: set,
		Materials: plan.Materials(),
	})
	return set, projection, err
}
