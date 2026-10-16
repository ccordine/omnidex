package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

var (
	ErrCodingPlanRevision = errors.New("coding plan revision conflict")
	ErrCodingPlanState    = errors.New("coding plan state conflict")
)

func (r *Repository) ApplyCodingPlanDecisions(
	ctx context.Context,
	command ApplyCodingPlanDecisionsCommand,
) (CodingPlanMutationResult, error) {
	command, err := normalizeApplyCodingPlanDecisionsCommand(command)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	descriptor, err := describeLifecycleOperation(
		command.OperationID, LifecycleCodingPlanDecisions, command,
	)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockLifecycleOperationIdentityTx(ctx, tx, command.OperationID); err != nil {
		return CodingPlanMutationResult{}, err
	}
	job, err := lockedJobTx(ctx, tx, command.JobID)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	if err := requireCodingPlanWorkspaceAuthority(
		job, command.WorkspaceRoot, command.WorkspaceIdentity,
	); err != nil {
		return CodingPlanMutationResult{}, err
	}
	if existing, found, err := loadLifecycleOperationTx(ctx, tx, descriptor, command.JobID); err != nil {
		return CodingPlanMutationResult{}, err
	} else if found {
		plan, err := readCodingPlanOperationResultTx(ctx, tx, existing)
		if err != nil {
			return CodingPlanMutationResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CodingPlanMutationResult{}, err
		}
		return CodingPlanMutationResult{Plan: plan, Job: existing.ResultJob}, nil
	}
	if job.CurrentGeneration != command.Generation || job.Status != model.JobStatusWaiting {
		return CodingPlanMutationResult{}, fmt.Errorf(
			"%w: job %d is not waiting on generation %d plan review",
			ErrCodingPlanState, job.ID, command.Generation,
		)
	}
	planStepID, revision, err := lockReviewCodingPlanTx(ctx, tx, command.JobID, command.Generation)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	if revision != command.Revision {
		return CodingPlanMutationResult{}, fmt.Errorf(
			"%w: coding plan revision is %d, received %d",
			ErrCodingPlanRevision, revision, command.Revision,
		)
	}
	changed := false
	for _, change := range command.Decisions {
		var current model.CodingPlanDecision
		err := tx.QueryRow(ctx, `
			SELECT decision FROM coding_plan_leaves
			WHERE job_id=$1 AND generation=$2 AND leaf_id=$3
			FOR UPDATE
		`, command.JobID, command.Generation, change.LeafID).Scan(&current)
		if err != nil {
			return CodingPlanMutationResult{}, fmt.Errorf(
				"coding plan decision leaf %q: %w", change.LeafID, err,
			)
		}
		if current == change.Decision {
			continue
		}
		result, err := tx.Exec(ctx, `
			UPDATE coding_plan_leaves
			SET decision=$4,decision_origin_generation=$2,updated_at=clock_timestamp()
			WHERE job_id=$1 AND generation=$2 AND leaf_id=$3 AND decision=$5
		`, command.JobID, command.Generation, change.LeafID, change.Decision, current)
		if err != nil {
			return CodingPlanMutationResult{}, err
		}
		if result.RowsAffected() != 1 {
			return CodingPlanMutationResult{}, fmt.Errorf(
				"%w: coding plan leaf %q changed during decision update",
				ErrCodingPlanRevision, change.LeafID,
			)
		}
		changed = true
	}
	if changed {
		result, err := tx.Exec(ctx, `
			UPDATE coding_plans
			SET revision=revision+1,updated_at=clock_timestamp()
			WHERE job_id=$1 AND generation=$2 AND revision=$3 AND state=$4
		`, command.JobID, command.Generation, command.Revision, model.CodingPlanStateReview)
		if err != nil {
			return CodingPlanMutationResult{}, err
		}
		if result.RowsAffected() != 1 {
			return CodingPlanMutationResult{}, fmt.Errorf("%w: coding plan changed during decision update", ErrCodingPlanRevision)
		}
		if _, err := tx.Exec(ctx, `UPDATE jobs SET updated_at=clock_timestamp() WHERE id=$1`, job.ID); err != nil {
			return CodingPlanMutationResult{}, err
		}
	}
	plan, err := readCodingPlan(ctx, tx, command.JobID, command.Generation)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	job, err = scanLockedJobTx(ctx, tx, command.JobID)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	stepStatus := model.StepStatusWaiting
	if err := insertLifecycleOperationTx(ctx, tx, descriptor, lifecycleOperationRecord{
		ID: descriptor.ID, JobID: job.ID,
		ObservedGeneration: command.Generation, ResultGeneration: command.Generation,
		StepID: &planStepID, Kind: descriptor.Kind,
		ResultJobStatus: job.Status, ResultStepStatus: &stepStatus, ResultJob: job,
	}); err != nil {
		return CodingPlanMutationResult{}, err
	}
	if err := insertCodingPlanOperationResultTx(ctx, tx, descriptor, plan); err != nil {
		return CodingPlanMutationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CodingPlanMutationResult{}, err
	}
	return CodingPlanMutationResult{Plan: plan, Job: job, Applied: changed}, nil
}

func (r *Repository) FreezeCodingPlan(
	ctx context.Context,
	command FreezeCodingPlanCommand,
) (CodingPlanMutationResult, error) {
	command, err := normalizeFreezeCodingPlanCommand(command)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	descriptor, err := describeLifecycleOperation(
		command.OperationID, LifecycleCodingPlanFreeze, command,
	)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockLifecycleOperationIdentityTx(ctx, tx, command.OperationID); err != nil {
		return CodingPlanMutationResult{}, err
	}
	job, err := lockedJobTx(ctx, tx, command.JobID)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	if err := requireCodingPlanWorkspaceAuthority(
		job, command.WorkspaceRoot, command.WorkspaceIdentity,
	); err != nil {
		return CodingPlanMutationResult{}, err
	}
	if existing, found, err := loadLifecycleOperationTx(ctx, tx, descriptor, command.JobID); err != nil {
		return CodingPlanMutationResult{}, err
	} else if found {
		plan, err := readCodingPlanOperationResultTx(ctx, tx, existing)
		if err != nil {
			return CodingPlanMutationResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return CodingPlanMutationResult{}, err
		}
		return CodingPlanMutationResult{Plan: plan, Job: existing.ResultJob}, nil
	}
	if job.CurrentGeneration != command.Generation || job.Status != model.JobStatusWaiting {
		return CodingPlanMutationResult{}, fmt.Errorf(
			"%w: job %d is not waiting on generation %d plan review",
			ErrCodingPlanState, job.ID, command.Generation,
		)
	}
	planStepID, revision, err := lockReviewCodingPlanTx(ctx, tx, command.JobID, command.Generation)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	if revision != command.Revision {
		return CodingPlanMutationResult{}, fmt.Errorf(
			"%w: coding plan revision is %d, received %d",
			ErrCodingPlanRevision, revision, command.Revision,
		)
	}
	var pending, approved, total int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE decision=$3),
		       COUNT(*) FILTER (WHERE decision=$4),COUNT(*)
		FROM coding_plan_leaves
		WHERE job_id=$1 AND generation=$2
	`, command.JobID, command.Generation,
		model.CodingPlanDecisionPending, model.CodingPlanDecisionApproved).Scan(&pending, &approved, &total); err != nil {
		return CodingPlanMutationResult{}, err
	}
	if pending != 0 {
		return CodingPlanMutationResult{}, fmt.Errorf(
			"%w: coding plan has %d undecided leaves", ErrCodingPlanState, pending,
		)
	}
	if approved == 0 {
		return CodingPlanMutationResult{}, fmt.Errorf(
			"%w: coding plan requires at least one approved leaf", ErrCodingPlanState,
		)
	}
	configuration, err := codingagent.ConfigurationFromMetadata(job.Metadata)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	if configuration.Mode == codingagent.Agents {
		return CodingPlanMutationResult{}, fmt.Errorf(
			"%w: Agents planning requires the normalized PostgreSQL coding ledger", ErrCodingPlanState,
		)
	}
	result, err := tx.Exec(ctx, `
		UPDATE coding_plans
		SET revision=revision+1,state=$4,updated_at=clock_timestamp(),frozen_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND revision=$3 AND state=$5
	`, command.JobID, command.Generation, command.Revision,
		model.CodingPlanStateFrozen, model.CodingPlanStateReview)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	if result.RowsAffected() != 1 {
		return CodingPlanMutationResult{}, fmt.Errorf("%w: coding plan changed during freeze", ErrCodingPlanRevision)
	}
	result, err = tx.Exec(ctx, `
		UPDATE job_steps
		SET status=$4,output=$5,finished_at=clock_timestamp(),updated_at=clock_timestamp()
		WHERE id=$1 AND job_id=$2 AND generation=$3 AND status=$6
		  AND superseded_at_generation IS NULL AND action='v3_coding_plan'
	`, planStepID, command.JobID, command.Generation, model.StepStatusCompleted,
		"coding plan approved and frozen", model.StepStatusWaiting)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	if result.RowsAffected() != 1 {
		return CodingPlanMutationResult{}, fmt.Errorf("%w: coding plan review step changed during freeze", ErrCodingPlanState)
	}
	result, err = tx.Exec(ctx, `
		UPDATE jobs
		SET status=$3,result=NULL,error=NULL,completed_at=NULL,updated_at=clock_timestamp()
		WHERE id=$1 AND current_generation=$2 AND status=$4
	`, command.JobID, command.Generation, model.JobStatusRunning, model.JobStatusWaiting)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	if result.RowsAffected() != 1 {
		return CodingPlanMutationResult{}, fmt.Errorf("%w: coding plan job changed during freeze", ErrCodingPlanState)
	}
	plan, err := readCodingPlan(ctx, tx, command.JobID, command.Generation)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	job, err = scanLockedJobTx(ctx, tx, command.JobID)
	if err != nil {
		return CodingPlanMutationResult{}, err
	}
	stepStatus := model.StepStatusCompleted
	if err := insertLifecycleOperationTx(ctx, tx, descriptor, lifecycleOperationRecord{
		ID: descriptor.ID, JobID: job.ID,
		ObservedGeneration: command.Generation, ResultGeneration: command.Generation,
		StepID: &planStepID, Kind: descriptor.Kind,
		ResultJobStatus: job.Status, ResultStepStatus: &stepStatus, ResultJob: job,
	}); err != nil {
		return CodingPlanMutationResult{}, err
	}
	if err := insertCodingPlanOperationResultTx(ctx, tx, descriptor, plan); err != nil {
		return CodingPlanMutationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CodingPlanMutationResult{}, err
	}
	return CodingPlanMutationResult{Plan: plan, Job: job, Applied: true}, nil
}

func lockReviewCodingPlanTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, generation int64,
) (int64, int64, error) {
	var stepID, revision int64
	var state model.CodingPlanState
	err := tx.QueryRow(ctx, `
		SELECT plan_step_id,revision,state
		FROM coding_plans
		WHERE job_id=$1 AND generation=$2
		FOR UPDATE
	`, jobID, generation).Scan(&stepID, &revision, &state)
	if err != nil {
		return 0, 0, err
	}
	if state != model.CodingPlanStateReview {
		return 0, 0, fmt.Errorf("%w: coding plan state is %q", ErrCodingPlanState, state)
	}
	var action, status string
	if err := tx.QueryRow(ctx, `
		SELECT action,status FROM job_steps
		WHERE id=$1 AND job_id=$2 AND generation=$3 AND superseded_at_generation IS NULL
		FOR UPDATE
	`, stepID, jobID, generation).Scan(&action, &status); err != nil {
		return 0, 0, err
	}
	if action != "v3_coding_plan" || status != model.StepStatusWaiting {
		return 0, 0, fmt.Errorf(
			"%w: coding plan step is %s/%s, expected v3_coding_plan/waiting_input",
			ErrCodingPlanState, action, status,
		)
	}
	return stepID, revision, nil
}

func (r *Repository) ApplyCodingLedgerMutation(ctx context.Context, command ApplyCodingLedgerMutationCommand) (CodingLedgerRevision, error) {
	if r == nil || r.pool == nil || command.LedgerID <= 0 || strings.TrimSpace(command.OwnerTurnText) == "" {
		return CodingLedgerRevision{}, fmt.Errorf("coding ledger mutation requires repository, ledger, and exact owner turn")
	}
	if err := command.Mutation.Validate(); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := validateCodingLedgerOperationID(command.Mutation.OperationID); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := validateCodingLedgerExternalOwnerTurnID(command.Mutation.OwnerTurnID); err != nil {
		return CodingLedgerRevision{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	defer tx.Rollback(ctx)

	var current int64
	var state string
	if err := tx.QueryRow(ctx, `SELECT current_revision,state FROM coding_ledgers WHERE id=$1 FOR UPDATE`, command.LedgerID).Scan(&current, &state); err != nil {
		return CodingLedgerRevision{}, err
	}
	var replayRevision, replayBase int64
	var replayOwner, replayText string
	var replayCoverage bool
	var replayMutation []byte
	err = tx.QueryRow(ctx, `
		SELECT revision.revision,revision.base_revision,revision.owner_turn_id,turn_record.exact_text,revision.coverage_passed,
		       revision.mutation_payload
		FROM coding_ledger_revisions revision
		JOIN coding_ledger_owner_turns turn_record
		  ON turn_record.ledger_id=revision.ledger_id AND turn_record.owner_turn_id=revision.owner_turn_id
		WHERE revision.ledger_id=$1 AND revision.operation_id=$2
	`, command.LedgerID, command.Mutation.OperationID).Scan(
		&replayRevision, &replayBase, &replayOwner, &replayText, &replayCoverage, &replayMutation,
	)
	if err == nil {
		expectedMutation, encodeErr := json.Marshal(command.Mutation)
		if encodeErr != nil {
			return CodingLedgerRevision{}, encodeErr
		}
		if replayBase != command.Mutation.BaseRevision || replayOwner != command.Mutation.OwnerTurnID ||
			replayText != command.OwnerTurnText || replayCoverage != command.CoveragePassed ||
			!sameCodingLedgerJSON(replayMutation, expectedMutation) {
			return CodingLedgerRevision{}, fmt.Errorf("coding ledger operation %q was already used by a different mutation", command.Mutation.OperationID)
		}
		result, readErr := readCodingLedgerRevisionTx(ctx, tx, command.LedgerID, replayRevision, CodingLedgerReadOptions{Limit: -1})
		if readErr != nil {
			return CodingLedgerRevision{}, readErr
		}
		matches, matchErr := codingLedgerMutationReplayMatchesTx(ctx, tx, command, result)
		if matchErr != nil {
			return CodingLedgerRevision{}, matchErr
		}
		if !matches {
			return CodingLedgerRevision{}, fmt.Errorf("coding ledger operation %q was already used by a different mutation", command.Mutation.OperationID)
		}
		if err := tx.Commit(ctx); err != nil {
			return CodingLedgerRevision{}, err
		}
		result.Applied = false
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CodingLedgerRevision{}, err
	}
	if state != "proposed" && state != "revising" {
		return CodingLedgerRevision{}, fmt.Errorf("%w: coding ledger state is %s", ErrCodingLedgerState, state)
	}
	if command.Mutation.BaseRevision != current {
		return CodingLedgerRevision{}, fmt.Errorf("%w: mutation base revision %d is stale; current revision is %d",
			ErrCodingLedgerRevision, command.Mutation.BaseRevision, current)
	}
	currentProjection, err := readCodingLedgerRevisionTx(ctx, tx, command.LedgerID, current, CodingLedgerReadOptions{Limit: -1})
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	next, err := cloneCodingLedger(currentProjection.Ledger)
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	next.Revision = current + 1
	if err := applyCodingLedgerOperations(&next, command.Mutation.Operations); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := validateDraftLedgerForRepository(next); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := validateCodingLedgerExactDuplicates(next); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := validateMutationOwnerTurnDisposition(
		currentProjection.Ledger, next, command.Mutation.Operations, command.Mutation.OwnerTurnDisposition,
	); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := validateCodingLedgerRouteMutation(
		currentProjection.Routes, command.Routes, next.Revision,
		command.Mutation.Operations, command.Mutation.OwnerTurnDisposition,
	); err != nil {
		return CodingLedgerRevision{}, err
	}
	routes := currentProjection.Routes
	if len(command.Routes) > 0 {
		routes = command.Routes
	}
	if err := validateCodingLedgerRoutes(routes, next.Revision); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := insertCodingLedgerOwnerTurnTx(ctx, tx, command.LedgerID, command.Mutation.OwnerTurnID,
		current, "planning", command.OwnerTurnText); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := closeCodingLedgerQuestionsTx(ctx, tx, command.LedgerID, command.Mutation.OwnerTurnID, next); err != nil {
		return CodingLedgerRevision{}, err
	}
	mutationPayload, err := json.Marshal(command.Mutation)
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO coding_ledger_revisions(
			ledger_id,revision,base_revision,operation_id,owner_turn_id,state,coverage_passed,mutation_payload
		) VALUES($1,$2,$3,$4,$5,'proposed',$6,$7::jsonb)
	`, command.LedgerID, next.Revision, current, command.Mutation.OperationID,
		command.Mutation.OwnerTurnID, command.CoveragePassed, string(mutationPayload)); err != nil {
		return CodingLedgerRevision{}, fmt.Errorf("create coding ledger revision: %w", err)
	}
	if err := persistCodingLedgerSnapshotTx(ctx, tx, command.LedgerID, next.Revision, current,
		command.Mutation.OwnerTurnID, next, routes); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := insertCodingLedgerDispositionTx(ctx, tx, command.LedgerID, next.Revision,
		command.Mutation.OwnerTurnID, command.Mutation.OwnerTurnDisposition, next, currentProjection.Ledger); err != nil {
		return CodingLedgerRevision{}, err
	}
	update, err := tx.Exec(ctx, `
		UPDATE coding_ledgers SET state='proposed',updated_at=clock_timestamp()
		WHERE id=$1 AND current_revision=$2 AND state IN ('proposed','revising')
	`, command.LedgerID, next.Revision)
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	if update.RowsAffected() != 1 {
		return CodingLedgerRevision{}, fmt.Errorf("%w: coding ledger changed during mutation", ErrCodingLedgerRevision)
	}
	result, err := readCodingLedgerRevisionTx(ctx, tx, command.LedgerID, next.Revision, CodingLedgerReadOptions{Limit: -1})
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CodingLedgerRevision{}, err
	}
	result.Applied = true
	return result, nil
}

func cloneCodingLedger(ledger model.EngineeringLedger) (model.EngineeringLedger, error) {
	raw, err := json.Marshal(ledger)
	if err != nil {
		return model.EngineeringLedger{}, fmt.Errorf("clone coding ledger: %w", err)
	}
	var cloned model.EngineeringLedger
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return model.EngineeringLedger{}, fmt.Errorf("clone coding ledger: %w", err)
	}
	return cloned, nil
}

func codingLedgerMutationReplayMatchesTx(
	ctx context.Context,
	tx pgx.Tx,
	command ApplyCodingLedgerMutationCommand,
	result CodingLedgerRevision,
) (bool, error) {
	base, err := readCodingLedgerRevisionTx(ctx, tx, command.LedgerID, command.Mutation.BaseRevision, CodingLedgerReadOptions{Limit: -1})
	if err != nil {
		return false, err
	}
	expectedLedger, err := cloneCodingLedger(base.Ledger)
	if err != nil {
		return false, err
	}
	expectedLedger.Revision = result.Revision
	if err := applyCodingLedgerOperations(&expectedLedger, command.Mutation.Operations); err != nil {
		return false, nil
	}
	resultLedger := result.Ledger
	sortCodingLedgerProjection(&expectedLedger)
	sortCodingLedgerProjection(&resultLedger)
	expectedRaw, err := json.Marshal(expectedLedger)
	if err != nil {
		return false, err
	}
	resultRaw, err := json.Marshal(resultLedger)
	if err != nil {
		return false, err
	}
	expectedRoutes := base.Routes
	if len(command.Routes) > 0 {
		expectedRoutes = command.Routes
	}
	if !bytes.Equal(expectedRaw, resultRaw) || !codingLedgerRoutesEqual(expectedRoutes, result.Routes) {
		return false, fmt.Errorf("coding ledger replay projection differs: ledger=%t routes=%t", bytes.Equal(expectedRaw, resultRaw), codingLedgerRoutesEqual(expectedRoutes, result.Routes))
	}
	return codingLedgerDispositionMatchesTx(
		ctx, tx, command.LedgerID, result.Revision,
		command.Mutation.OwnerTurnID, command.Mutation.OwnerTurnDisposition, expectedLedger, base.Ledger,
	)
}

func codingLedgerDispositionMatchesTx(
	ctx context.Context,
	tx pgx.Tx,
	ledgerID, revision int64,
	ownerTurnID string,
	expectedDisposition model.OwnerTurnDisposition,
	ledger model.EngineeringLedger,
	authorityLedgers ...model.EngineeringLedger,
) (bool, error) {
	var rawStatements []byte
	if err := tx.QueryRow(ctx, `
		SELECT statements
		FROM coding_ledger_owner_turn_dispositions
		WHERE ledger_id=$1 AND owner_turn_id=$2 AND revision=$3
	`, ledgerID, ownerTurnID, revision).Scan(&rawStatements); err != nil {
		return false, err
	}
	var storedStatements []model.OwnerStatementDisposition
	if err := json.Unmarshal(rawStatements, &storedStatements); err != nil {
		return false, err
	}
	rows, err := tx.Query(ctx, `
		SELECT item_id FROM coding_ledger_owner_turn_items
		WHERE ledger_id=$1 AND owner_turn_id=$2 ORDER BY item_id
	`, ledgerID, ownerTurnID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	storedIDs := make([]model.LedgerItemID, 0)
	for rows.Next() {
		var id model.LedgerItemID
		if err := rows.Scan(&id); err != nil {
			return false, err
		}
		storedIDs = append(storedIDs, id)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	expectedStatements := expectedDisposition.StatementDispositions()
	expectedIDs := make([]model.LedgerItemID, 0)
	for _, statement := range expectedStatements {
		expectedIDs = append(expectedIDs, statement.ItemIDs...)
		expectedIDs = append(expectedIDs, statement.QuestionIDs...)
	}
	dependencyByID := make(map[model.LedgerItemID][]model.LedgerDependency)
	ledgers := append([]model.EngineeringLedger{ledger}, authorityLedgers...)
	for _, authorityLedger := range ledgers {
		for _, dependency := range authorityLedger.Dependencies {
			dependencyByID[dependency.ID] = append(dependencyByID[dependency.ID], dependency)
		}
	}
	persistedSet := make(map[model.LedgerItemID]bool)
	for _, id := range expectedIDs {
		if dependencies := dependencyByID[id]; len(dependencies) > 0 {
			for _, dependency := range dependencies {
				persistedSet[dependency.TaskID] = true
				persistedSet[dependency.PrerequisiteID] = true
			}
		} else {
			persistedSet[id] = true
		}
	}
	expectedIDs = expectedIDs[:0]
	for id := range persistedSet {
		expectedIDs = append(expectedIDs, id)
	}
	sort.Slice(expectedIDs, func(i, j int) bool { return expectedIDs[i] < expectedIDs[j] })
	return reflect.DeepEqual(storedStatements, expectedStatements) && sameCodingLedgerItemIDs(storedIDs, expectedIDs), nil
}

func sameCodingLedgerItemIDs(left, right []model.LedgerItemID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func applyCodingLedgerOperations(ledger *model.EngineeringLedger, operations []model.LedgerMutationOperation) error {
	removed := make(map[model.LedgerItemID]bool)
	restored := make(map[model.LedgerItemID]bool)
	for _, operation := range operations {
		switch operation.Kind {
		case model.LedgerMutationRemove:
			if restored[operation.TargetID] {
				return fmt.Errorf("coding ledger item %s cannot be removed and restored in one revision", operation.TargetID)
			}
			removed[operation.TargetID] = true
		case model.LedgerMutationRestore:
			if removed[operation.TargetID] {
				return fmt.Errorf("coding ledger item %s cannot be removed and restored in one revision", operation.TargetID)
			}
			restored[operation.TargetID] = true
		}
	}
	for _, operation := range operations {
		switch operation.Kind {
		case model.LedgerMutationAdd:
			if ledgerHasItem(*ledger, ledgerMutationItemID(operation.Item)) {
				return fmt.Errorf("coding ledger item %s already exists", ledgerMutationItemID(operation.Item))
			}
			appendCodingLedgerMutationItem(ledger, operation.Item)
			resetCodingLedgerAffectedAgreements(ledger, ledgerMutationItemID(operation.Item))
		case model.LedgerMutationAmend:
			if disposition, found := codingLedgerItemDisposition(*ledger, operation.TargetID); !found || disposition != model.LedgerDispositionActive {
				return fmt.Errorf("coding ledger amend target %s is not active", operation.TargetID)
			}
			resetCodingLedgerAffectedAgreements(ledger, operation.TargetID)
			if !replaceCodingLedgerMutationItem(ledger, operation.TargetID, operation.Item) {
				return fmt.Errorf("coding ledger amend target %s does not exist with the same kind", operation.TargetID)
			}
			resetCodingLedgerAffectedAgreements(ledger, operation.TargetID)
			setCodingLedgerItem(ledger, operation.TargetID, func(item *model.LedgerItem) {
				item.Disposition = model.LedgerDispositionActive
				item.ReviewState = model.LedgerReviewProposed
			})
		case model.LedgerMutationRemove:
			if disposition, found := codingLedgerItemDisposition(*ledger, operation.TargetID); !found || disposition != model.LedgerDispositionActive {
				return fmt.Errorf("coding ledger remove target %s is not active", operation.TargetID)
			}
			resetCodingLedgerAffectedAgreements(ledger, operation.TargetID)
			if !setCodingLedgerItem(ledger, operation.TargetID, func(item *model.LedgerItem) {
				if item.Disposition != model.LedgerDispositionActive {
					return
				}
				item.Disposition = model.LedgerDispositionTombstoned
				item.ReviewState = ""
			}) {
				return fmt.Errorf("coding ledger remove target %s does not exist", operation.TargetID)
			}
		case model.LedgerMutationRestore:
			if disposition, found := codingLedgerItemDisposition(*ledger, operation.TargetID); !found || disposition != model.LedgerDispositionTombstoned {
				return fmt.Errorf("coding ledger restore target %s is not tombstoned", operation.TargetID)
			}
			if !setCodingLedgerItem(ledger, operation.TargetID, func(item *model.LedgerItem) {
				if item.Disposition != model.LedgerDispositionTombstoned {
					return
				}
				item.Disposition = model.LedgerDispositionActive
				item.ReviewState = model.LedgerReviewProposed
			}) {
				return fmt.Errorf("coding ledger restore target %s does not exist", operation.TargetID)
			}
			resetCodingLedgerAffectedAgreements(ledger, operation.TargetID)
		case model.LedgerMutationReview:
			if disposition, found := codingLedgerItemDisposition(*ledger, operation.TargetID); !found || disposition != model.LedgerDispositionActive {
				return fmt.Errorf("coding ledger review target %s is not active", operation.TargetID)
			}
			if !setCodingLedgerItem(ledger, operation.TargetID, func(item *model.LedgerItem) {
				if item.Disposition == model.LedgerDispositionActive {
					item.ReviewState = operation.ReviewState
				}
			}) {
				return fmt.Errorf("coding ledger review target %s does not exist", operation.TargetID)
			}
		case model.LedgerMutationReorder:
			if disposition, found := codingLedgerItemDisposition(*ledger, operation.TargetID); !found || disposition != model.LedgerDispositionActive {
				return fmt.Errorf("coding ledger reorder target %s is not active", operation.TargetID)
			}
			if !setCodingLedgerItem(ledger, operation.TargetID, func(item *model.LedgerItem) { item.Rank = *operation.Rank }) {
				return fmt.Errorf("coding ledger reorder target %s does not exist", operation.TargetID)
			}
		default:
			return fmt.Errorf("unsupported coding ledger mutation operation %q", operation.Kind)
		}
	}
	return nil
}

func resetCodingLedgerAffectedAgreements(ledger *model.EngineeringLedger, changedID model.LedgerItemID) {
	setCodingLedgerItem(ledger, changedID, func(item *model.LedgerItem) {
		if item.Disposition == model.LedgerDispositionActive {
			item.ReviewState = model.LedgerReviewProposed
		}
	})

	// A required check is part of the agreement for every criterion and task
	// that names its verification scope. Amend calls this before and after
	// replacement so both the prior and replacement scopes are reset.
	for _, check := range ledger.Checks {
		if check.ID != changedID || check.Disposition != model.LedgerDispositionActive || !check.Required {
			continue
		}
		criterionIDs := make(map[model.LedgerItemID]bool, len(check.AcceptanceCriterionIDs))
		for _, criterionID := range check.AcceptanceCriterionIDs {
			criterionIDs[criterionID] = true
			setCodingLedgerItem(ledger, criterionID, func(item *model.LedgerItem) {
				if item.Disposition == model.LedgerDispositionActive {
					item.ReviewState = model.LedgerReviewProposed
				}
			})
		}
		requirementIDs := make(map[model.LedgerItemID]bool, len(check.RequirementIDs))
		for _, requirementID := range check.RequirementIDs {
			requirementIDs[requirementID] = true
		}
		for index := range ledger.Tasks {
			task := &ledger.Tasks[index]
			if task.Disposition != model.LedgerDispositionActive {
				continue
			}
			deliveryOnly := len(task.RequirementIDs) == 0 && strings.TrimSpace(task.DeliveryObligation) != ""
			if (deliveryOnly || ledgerItemIDsIntersect(task.RequirementIDs, requirementIDs)) &&
				ledgerItemIDsIntersect(task.AcceptanceCriterionIDs, criterionIDs) {
				task.ReviewState = model.LedgerReviewProposed
			}
		}
	}

	// A dependency changes the meaning of its dependent task. Propagate that
	// reset through active task dependencies, but preserve every unrelated
	// agreement byte-for-byte.
	pending := []model.LedgerItemID{changedID}
	seen := map[model.LedgerItemID]bool{changedID: true}
	for len(pending) > 0 {
		current := pending[0]
		pending = pending[1:]
		for index := range ledger.Dependencies {
			dependency := &ledger.Dependencies[index]
			if dependency.Disposition != model.LedgerDispositionActive && dependency.ID != changedID {
				continue
			}
			if dependency.ID != current && dependency.PrerequisiteID != current {
				continue
			}
			if dependency.Disposition == model.LedgerDispositionActive {
				dependency.ReviewState = model.LedgerReviewProposed
			}
			if !seen[dependency.TaskID] {
				seen[dependency.TaskID] = true
				pending = append(pending, dependency.TaskID)
				setCodingLedgerItem(ledger, dependency.TaskID, func(item *model.LedgerItem) {
					if item.Disposition == model.LedgerDispositionActive {
						item.ReviewState = model.LedgerReviewProposed
					}
				})
			}
		}
	}
}

func ledgerItemIDsIntersect(ids []model.LedgerItemID, candidates map[model.LedgerItemID]bool) bool {
	for _, id := range ids {
		if candidates[id] {
			return true
		}
	}
	return false
}

func ledgerMutationItemID(item *model.LedgerMutationItem) model.LedgerItemID {
	switch {
	case item == nil:
		return ""
	case item.Objective != nil:
		return item.Objective.ID
	case item.Requirement != nil:
		return item.Requirement.ID
	case item.Constraint != nil:
		return item.Constraint.ID
	case item.Group != nil:
		return item.Group.ID
	case item.Task != nil:
		return item.Task.ID
	case item.AcceptanceCriterion != nil:
		return item.AcceptanceCriterion.ID
	case item.Check != nil:
		return item.Check.ID
	case item.DecisionQuestion != nil:
		return item.DecisionQuestion.ID
	case item.Dependency != nil:
		return item.Dependency.ID
	case item.Finding != nil:
		return item.Finding.ID
	default:
		return ""
	}
}

func ledgerHasItem(ledger model.EngineeringLedger, id model.LedgerItemID) bool {
	found := false
	setCodingLedgerItem(&ledger, id, func(*model.LedgerItem) { found = true })
	return found
}

func appendCodingLedgerMutationItem(ledger *model.EngineeringLedger, item *model.LedgerMutationItem) {
	switch {
	case item.Objective != nil:
		ledger.Objectives = append(ledger.Objectives, *item.Objective)
	case item.Requirement != nil:
		ledger.Requirements = append(ledger.Requirements, *item.Requirement)
	case item.Constraint != nil:
		ledger.Constraints = append(ledger.Constraints, *item.Constraint)
	case item.Group != nil:
		ledger.Groups = append(ledger.Groups, *item.Group)
	case item.Task != nil:
		ledger.Tasks = append(ledger.Tasks, *item.Task)
	case item.AcceptanceCriterion != nil:
		ledger.AcceptanceCriteria = append(ledger.AcceptanceCriteria, *item.AcceptanceCriterion)
	case item.Check != nil:
		ledger.Checks = append(ledger.Checks, *item.Check)
	case item.DecisionQuestion != nil:
		ledger.DecisionsAndQuestions = append(ledger.DecisionsAndQuestions, *item.DecisionQuestion)
	case item.Dependency != nil:
		ledger.Dependencies = append(ledger.Dependencies, *item.Dependency)
	case item.Finding != nil:
		ledger.Findings = append(ledger.Findings, *item.Finding)
	}
}

func replaceCodingLedgerMutationItem(ledger *model.EngineeringLedger, target model.LedgerItemID, item *model.LedgerMutationItem) bool {
	switch {
	case item.Objective != nil:
		for index := range ledger.Objectives {
			if ledger.Objectives[index].ID == target {
				ledger.Objectives[index] = *item.Objective
				return true
			}
		}
	case item.Requirement != nil:
		for index := range ledger.Requirements {
			if ledger.Requirements[index].ID == target {
				ledger.Requirements[index] = *item.Requirement
				return true
			}
		}
	case item.Constraint != nil:
		for index := range ledger.Constraints {
			if ledger.Constraints[index].ID == target {
				ledger.Constraints[index] = *item.Constraint
				return true
			}
		}
	case item.Group != nil:
		for index := range ledger.Groups {
			if ledger.Groups[index].ID == target {
				ledger.Groups[index] = *item.Group
				return true
			}
		}
	case item.Task != nil:
		for index := range ledger.Tasks {
			if ledger.Tasks[index].ID == target {
				ledger.Tasks[index] = *item.Task
				return true
			}
		}
	case item.AcceptanceCriterion != nil:
		for index := range ledger.AcceptanceCriteria {
			if ledger.AcceptanceCriteria[index].ID == target {
				ledger.AcceptanceCriteria[index] = *item.AcceptanceCriterion
				return true
			}
		}
	case item.Check != nil:
		for index := range ledger.Checks {
			if ledger.Checks[index].ID == target {
				ledger.Checks[index] = *item.Check
				return true
			}
		}
	case item.DecisionQuestion != nil:
		for index := range ledger.DecisionsAndQuestions {
			if ledger.DecisionsAndQuestions[index].ID == target {
				if ledger.DecisionsAndQuestions[index].Kind != item.DecisionQuestion.Kind {
					return false
				}
				ledger.DecisionsAndQuestions[index] = *item.DecisionQuestion
				return true
			}
		}
	case item.Dependency != nil:
		for index := range ledger.Dependencies {
			if ledger.Dependencies[index].ID == target {
				ledger.Dependencies[index] = *item.Dependency
				return true
			}
		}
	case item.Finding != nil:
		for index := range ledger.Findings {
			if ledger.Findings[index].ID == target {
				ledger.Findings[index] = *item.Finding
				return true
			}
		}
	}
	return false
}

func setCodingLedgerItem(ledger *model.EngineeringLedger, id model.LedgerItemID, update func(*model.LedgerItem)) bool {
	for index := range ledger.Objectives {
		if ledger.Objectives[index].ID == id {
			update(&ledger.Objectives[index].LedgerItem)
			return true
		}
	}
	for index := range ledger.Requirements {
		if ledger.Requirements[index].ID == id {
			update(&ledger.Requirements[index].LedgerItem)
			return true
		}
	}
	for index := range ledger.Constraints {
		if ledger.Constraints[index].ID == id {
			update(&ledger.Constraints[index].LedgerItem)
			return true
		}
	}
	for index := range ledger.Groups {
		if ledger.Groups[index].ID == id {
			update(&ledger.Groups[index].LedgerItem)
			return true
		}
	}
	for index := range ledger.Tasks {
		if ledger.Tasks[index].ID == id {
			update(&ledger.Tasks[index].LedgerItem)
			return true
		}
	}
	for index := range ledger.AcceptanceCriteria {
		if ledger.AcceptanceCriteria[index].ID == id {
			update(&ledger.AcceptanceCriteria[index].LedgerItem)
			return true
		}
	}
	for index := range ledger.Checks {
		if ledger.Checks[index].ID == id {
			update(&ledger.Checks[index].LedgerItem)
			return true
		}
	}
	for index := range ledger.DecisionsAndQuestions {
		if ledger.DecisionsAndQuestions[index].ID == id {
			update(&ledger.DecisionsAndQuestions[index].LedgerItem)
			return true
		}
	}
	for index := range ledger.Dependencies {
		if ledger.Dependencies[index].ID == id {
			update(&ledger.Dependencies[index].LedgerItem)
			return true
		}
	}
	for index := range ledger.Findings {
		if ledger.Findings[index].ID == id {
			update(&ledger.Findings[index].LedgerItem)
			return true
		}
	}
	return false
}

func codingLedgerItemDisposition(ledger model.EngineeringLedger, id model.LedgerItemID) (model.LedgerDisposition, bool) {
	var disposition model.LedgerDisposition
	found := setCodingLedgerItem(&ledger, id, func(item *model.LedgerItem) { disposition = item.Disposition })
	return disposition, found
}
