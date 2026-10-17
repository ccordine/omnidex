package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (repository *Repository) FrozenCodingLedgerConfiguration(
	ctx context.Context,
	authority model.StepAttemptAuthority,
) (codingagent.Configuration, error) {
	if repository == nil || repository.pool == nil {
		return codingagent.Configuration{}, fmt.Errorf("frozen coding ledger configuration requires repository authority")
	}
	// The queue's active-attempt authority check intentionally locks the exact
	// attempt. Route selection must share that authoritative transaction rather
	// than weakening it to a non-locking read.
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return codingagent.Configuration{}, err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return codingagent.Configuration{}, err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return codingagent.Configuration{}, staleStepAttemptError(authority, "coding ledger route selection requires current execution", nil)
	}
	var action string
	if err := tx.QueryRow(ctx, `
		SELECT action FROM job_steps
		WHERE id=$1 AND job_id=$2 AND generation=$3
	`, authority.StepID, authority.JobID, authority.Generation).Scan(&action); err != nil {
		return codingagent.Configuration{}, err
	}
	if action != "v3_coding" {
		return codingagent.Configuration{}, fmt.Errorf("frozen coding ledger routes require the execution step")
	}
	configuration, err := frozenCodingLedgerConfigurationTx(ctx, tx, authority.JobID)
	if err != nil {
		return codingagent.Configuration{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return codingagent.Configuration{}, err
	}
	return configuration, nil
}

func frozenCodingLedgerConfigurationTx(ctx context.Context, tx pgx.Tx, jobID int64) (codingagent.Configuration, error) {
	var metadata json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT metadata FROM jobs WHERE id=$1`, jobID).Scan(&metadata); err != nil {
		return codingagent.Configuration{}, err
	}
	persisted, err := codingagent.ConfigurationFromMetadata(metadata)
	if err != nil {
		return codingagent.Configuration{}, err
	}
	if persisted.LedgerParser == (codingagent.Route{}) {
		return codingagent.Configuration{}, fmt.Errorf("frozen coding ledger lacks its immutable job parser route")
	}
	rows, err := tx.Query(ctx, `
		SELECT route.role,route.provider,route.model,route.reasoning_effort,route.service_tier
		FROM coding_ledgers ledger
		JOIN coding_ledger_routes route
		  ON route.ledger_id=ledger.id AND route.revision=ledger.frozen_revision
		WHERE ledger.job_id=$1 AND ledger.state='frozen' AND ledger.frozen_revision IS NOT NULL
		ORDER BY route.role
	`, jobID)
	if err != nil {
		return codingagent.Configuration{}, err
	}
	defer rows.Close()
	configuration := codingagent.Configuration{Mode: codingagent.Agents, LedgerParser: persisted.LedgerParser}
	count := 0
	for rows.Next() {
		var role string
		var route codingagent.Route
		if err := rows.Scan(&role, &route.Provider, &route.Model, &route.Effort, &route.ServiceTier); err != nil {
			return codingagent.Configuration{}, err
		}
		switch role {
		case "planner":
			configuration.Planner = route
		case "executor":
			configuration.Executor = route
		case "reviewer":
			configuration.Reviewer = route
		default:
			return codingagent.Configuration{}, fmt.Errorf("frozen coding ledger has unknown route %q", role)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return codingagent.Configuration{}, err
	}
	if count != 3 {
		return codingagent.Configuration{}, fmt.Errorf("frozen coding ledger requires exactly three routes")
	}
	return configuration, configuration.Validate()
}

// NextIncompleteCodingLedgerTask is the execution scheduler for a frozen task
// ledger. The database chooses one unfinished task whose prerequisites have
// already been accepted. The caller executes and independently reviews only
// the returned task, records task_accepted, and asks again.
func (repository *Repository) NextIncompleteCodingLedgerTask(
	ctx context.Context,
	authority model.StepAttemptAuthority,
) (*codingagent.Task, error) {
	if repository == nil || repository.pool == nil {
		return nil, fmt.Errorf("next coding ledger task requires repository authority")
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return nil, err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return nil, staleStepAttemptError(authority, "coding ledger task selection requires current execution", nil)
	}
	var action string
	if err := tx.QueryRow(ctx, `
		SELECT action FROM job_steps
		WHERE id=$1 AND job_id=$2 AND generation=$3
	`, authority.StepID, authority.JobID, authority.Generation).Scan(&action); err != nil {
		return nil, err
	}
	if action != "v3_coding" {
		return nil, fmt.Errorf("coding ledger task selection requires the execution step")
	}
	var ledgerID, revision int64
	var taskID model.LedgerItemID
	err = tx.QueryRow(ctx, `
		SELECT ledger.id,ledger.frozen_revision,task.task_item_id
		FROM coding_ledgers ledger
		JOIN coding_agent_tasks task
		  ON task.ledger_id=ledger.id
		 AND task.frozen_revision=ledger.frozen_revision
		JOIN coding_ledger_revision_items projection
		  ON projection.ledger_id=task.ledger_id
		 AND projection.revision=task.frozen_revision
		 AND projection.item_id=task.task_item_id
		JOIN coding_ledger_item_versions version
		  ON version.ledger_id=projection.ledger_id
		 AND version.item_id=projection.item_id
		 AND version.revision=projection.item_revision
		WHERE ledger.job_id=$1
		  AND ledger.state='frozen'
		  AND ledger.frozen_revision IS NOT NULL
		  AND task.state='ready'
		ORDER BY version.rank,task.fairness_key
		LIMIT 1
	`, authority.JobID).Scan(&ledgerID, &revision, &taskID)
	if err != nil {
		if err == pgx.ErrNoRows {
			var incomplete bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS(
				    SELECT 1
				    FROM coding_ledgers ledger
				    JOIN coding_agent_tasks task
				      ON task.ledger_id=ledger.id
				     AND task.frozen_revision=ledger.frozen_revision
				    WHERE ledger.job_id=$1
				      AND ledger.state='frozen'
				      AND task.state NOT IN ('accepted','cancelled')
				)
			`, authority.JobID).Scan(&incomplete); err != nil {
				return nil, err
			}
			if incomplete {
				return nil, fmt.Errorf("coding task ledger has incomplete tasks but none are dependency-ready")
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return nil, nil
		}
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	detail, err := repository.completeCodingLedgerTaskDetail(ctx, ledgerID, revision, taskID)
	if err != nil {
		return nil, err
	}
	return codingAgentTaskFromLedgerDetail(detail)
}

func (repository *Repository) AcceptCodingLedgerTask(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	taskID string,
) error {
	if repository == nil || repository.pool == nil || strings.TrimSpace(taskID) == "" {
		return fmt.Errorf("coding ledger task acceptance requires repository and task authority")
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	jobStatus, stepStatus, _, err := requireActiveStepAttemptTx(ctx, tx, authority)
	if err != nil {
		return err
	}
	if jobStatus != model.JobStatusRunning || stepStatus != model.StepStatusRunning {
		return staleStepAttemptError(authority, "coding ledger task acceptance requires current execution", nil)
	}
	var action string
	if err := tx.QueryRow(ctx, `
		SELECT action FROM job_steps
		WHERE id=$1 AND job_id=$2 AND generation=$3
	`, authority.StepID, authority.JobID, authority.Generation).Scan(&action); err != nil {
		return err
	}
	if action != "v3_coding" {
		return fmt.Errorf("coding ledger task acceptance requires the execution step")
	}
	var ledgerID, revision int64
	var state string
	err = tx.QueryRow(ctx, `
		SELECT task.ledger_id,task.frozen_revision,task.state
		FROM coding_agent_tasks task
		JOIN coding_ledgers ledger ON ledger.id=task.ledger_id AND ledger.frozen_revision=task.frozen_revision
		WHERE ledger.job_id=$1 AND ledger.state='frozen' AND task.task_item_id=$2
		FOR UPDATE OF task
	`, authority.JobID, taskID).Scan(&ledgerID, &revision, &state)
	if err != nil {
		return err
	}
	if state == "accepted" {
		return tx.Commit(ctx)
	}
	if state != "ready" {
		return fmt.Errorf("coding ledger task %s is %s instead of ready", taskID, state)
	}
	detail, err := readCodingLedgerRevisionTx(ctx, tx, ledgerID, revision, CodingLedgerReadOptions{
		Limit: -1, RelatedTaskID: model.LedgerItemID(taskID),
	})
	if err != nil {
		return err
	}
	task, err := codingAgentTaskFromLedgerDetail(detail)
	if err != nil {
		return err
	}
	configuration, err := frozenCodingLedgerConfigurationTx(ctx, tx, authority.JobID)
	if err != nil {
		return err
	}
	entries, err := codingAgentCompletionEntriesTx(ctx, tx, authority.JobID, authority.Generation)
	if err != nil {
		return err
	}
	if err := validateCodingTaskAcceptanceJournal(entries, authority.Generation, configuration, *task); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `
		UPDATE coding_agent_tasks
		SET state='accepted',state_version=state_version+1,
		    accepted_at=clock_timestamp(),cancelled_at=NULL,updated_at=clock_timestamp()
		WHERE ledger_id=$1 AND frozen_revision=$2 AND task_item_id=$3 AND state='ready'
	`, ledgerID, revision, taskID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("coding ledger task %s changed before acceptance", taskID)
	}
	return tx.Commit(ctx)
}

func (repository *Repository) completeCodingLedgerTaskDetail(ctx context.Context, ledgerID, revision int64, taskID model.LedgerItemID) (CodingLedgerRevision, error) {
	return repository.CodingLedgerTaskDetail(ctx, ledgerID, revision, taskID)
}

func codingAgentTaskFromLedgerDetail(detail CodingLedgerRevision) (*codingagent.Task, error) {
	if len(detail.Ledger.Tasks) != 1 {
		return nil, fmt.Errorf("coding ledger task selection requires exactly one task")
	}
	return codingAgentTaskFromLedger(detail.Ledger, detail.Ledger.Tasks[0].ID)
}

func codingAgentTaskFromLedger(ledger model.EngineeringLedger, id model.LedgerItemID) (*codingagent.Task, error) {
	for _, item := range ledger.Tasks {
		if item.ID == id && item.Disposition == model.LedgerDispositionActive {
			task := &codingagent.Task{ID: string(item.ID), Goal: item.Goal, ReadFiles: item.ReadFiles, WriteFiles: item.WriteFiles, DeleteFiles: item.DeleteFiles}
			return task, task.Validate()
		}
	}
	return nil, fmt.Errorf("active coding task %s not found", id)
}
func validateFrozenCodingAgentTasks(ledger model.EngineeringLedger) error {
	for _, task := range ledger.Tasks {
		if task.Disposition == model.LedgerDispositionActive {
			if _, err := codingAgentTaskFromLedger(ledger, task.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Repository) readCompleteCodingLedger(
	ctx context.Context,
	ledgerID, revision int64,
) (CodingLedgerRevision, error) {
	var result CodingLedgerRevision
	for offset := 0; ; offset += 1000 {
		page, err := r.ReadCodingLedger(ctx, ledgerID, CodingLedgerReadOptions{
			Revision: revision, Limit: 1000, Offset: offset,
		})
		if err != nil {
			return CodingLedgerRevision{}, err
		}
		if offset == 0 {
			result = page
			revision = page.Revision
			result.Ledger = model.EngineeringLedger{ID: page.Ledger.ID, Revision: page.Ledger.Revision}
		} else if page.Revision != result.Revision {
			return CodingLedgerRevision{}, fmt.Errorf("coding ledger changed while loading complete revision")
		}
		result.Ledger.Tasks = append(result.Ledger.Tasks, page.Ledger.Tasks...)
		count := len(ledgerRepositoryRecords(page.Ledger))
		if count < 1000 {
			return result, nil
		}
	}
}
