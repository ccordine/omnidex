package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

const (
	maxTaskLedgerNodes             = taskstate.MaxLedgerNodes
	maxTaskLedgerNodeRefs          = taskstate.MaxLedgerNodeVerificationRefs
	maxTaskLedgerEdges             = taskstate.MaxLedgerEdges
	maxTaskLedgerEntries           = taskstate.MaxLedgerEntries
	maxTaskLedgerRefs              = taskstate.MaxLedgerEntryRefs
	maxTaskLedgerNodeSupersessions = taskstate.MaxLedgerNodeSupersessions
)

func (r *Repository) TaskLedger(ctx context.Context, jobID int64) (taskstate.MaterializedState, error) {
	if err := validateTaskLedgerRequest(r, ctx, jobID); err != nil {
		return taskstate.MaterializedState{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return taskstate.MaterializedState{}, fmt.Errorf("begin task ledger read: %w", err)
	}
	defer tx.Rollback(ctx)
	ledger, err := loadTaskLedgerTx(ctx, tx, jobID, false)
	if err != nil {
		return taskstate.MaterializedState{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return taskstate.MaterializedState{}, fmt.Errorf("commit task ledger read for job %d: %w", jobID, err)
	}
	return ledger.MaterializedState(), nil
}

func validateTaskLedgerRequest(r *Repository, ctx context.Context, jobID int64) error {
	if ctx == nil {
		return fmt.Errorf("task ledger context is required")
	}
	if jobID <= 0 {
		return fmt.Errorf("task ledger job ID must be positive")
	}
	if r == nil || r.pool == nil {
		return fmt.Errorf("postgres repository is not configured")
	}
	return nil
}

func loadTaskLedgerTx(ctx context.Context, tx pgx.Tx, jobID int64, lock bool) (*taskstate.Ledger, error) {
	header, err := loadTaskLedgerHeaderTx(ctx, tx, jobID, lock)
	if err != nil {
		return nil, err
	}
	return restoreTaskLedgerTx(ctx, tx, header)
}

type taskLedgerHeader struct {
	ID         taskstate.LedgerID
	Owner      taskstate.LedgerOwner
	JobStatus  string
	Generation int64
	Version    uint64
	Status     taskstate.LedgerStatus
}

func loadTaskLedgerHeaderTx(ctx context.Context, tx pgx.Tx, jobID int64, lock bool) (taskLedgerHeader, error) {
	jobQuery := `SELECT COALESCE(metadata ->> 'telemetry_run_id', ''), current_generation, status FROM jobs WHERE id=$1`
	if lock {
		jobQuery += " FOR UPDATE"
	}
	var metadataRunID string
	var generation int64
	var jobStatus string
	if err := tx.QueryRow(ctx, jobQuery, jobID).Scan(&metadataRunID, &generation, &jobStatus); errors.Is(err, pgx.ErrNoRows) {
		return taskLedgerHeader{}, fmt.Errorf("%w: task ledger job %d", taskstate.ErrNotFound, jobID)
	} else if err != nil {
		return taskLedgerHeader{}, fmt.Errorf("lock task ledger job %d: %w", jobID, err)
	}
	ledgerQuery := `
		SELECT id, run_id::text, owner_type, owner_id, version, status
		FROM task_ledgers
		WHERE job_id=$1
	`
	if lock {
		ledgerQuery += " FOR UPDATE"
	}
	var ledgerID, runID, ownerType, status string
	var ownerID, version int64
	if err := tx.QueryRow(ctx, ledgerQuery, jobID).Scan(
		&ledgerID, &runID, &ownerType, &ownerID, &version, &status,
	); errors.Is(err, pgx.ErrNoRows) {
		return taskLedgerHeader{}, fmt.Errorf("%w: task ledger for job %d", taskstate.ErrNotFound, jobID)
	} else if err != nil {
		return taskLedgerHeader{}, fmt.Errorf("read task ledger for job %d: %w", jobID, err)
	}
	if ownerType != string(taskstate.OwnerJob) || ownerID != jobID || metadataRunID != runID ||
		generation <= 0 || version < 0 {
		return taskLedgerHeader{}, fmt.Errorf("%w: task ledger authority for job %d is inconsistent", taskstate.ErrInvalidState, jobID)
	}
	owner := taskstate.LedgerOwner{Kind: taskstate.OwnerJob, JobID: jobID, RunID: runID}
	expectedID, err := taskstate.NewLedgerID(owner)
	if err != nil || string(expectedID) != ledgerID {
		return taskLedgerHeader{}, fmt.Errorf("%w: task ledger identity for job %d is inconsistent", taskstate.ErrInvalidState, jobID)
	}
	return taskLedgerHeader{
		ID: expectedID, Owner: owner, JobStatus: jobStatus, Generation: generation,
		Version: uint64(version), Status: taskstate.LedgerStatus(status),
	}, nil
}

func restoreTaskLedgerTx(ctx context.Context, tx pgx.Tx, header taskLedgerHeader) (*taskstate.Ledger, error) {
	var err error
	state := taskstate.MaterializedState{
		ID: header.ID, Owner: header.Owner, Version: header.Version, Status: header.Status,
		Nodes: make([]taskstate.Node, 0), Edges: make([]taskstate.Edge, 0),
		Entries:           make([]taskstate.Entry, 0),
		NodeSupersessions: make([]taskstate.NodeGenerationSupersession, 0),
	}
	if state.Nodes, err = loadTaskLedgerNodes(ctx, tx, header.ID); err != nil {
		return nil, err
	}
	if state.Edges, err = loadTaskLedgerEdges(ctx, tx, header.ID); err != nil {
		return nil, err
	}
	if state.Entries, err = loadTaskLedgerEntries(ctx, tx, header.ID); err != nil {
		return nil, err
	}
	if state.NodeSupersessions, err = loadTaskLedgerNodeSupersessions(ctx, tx, header.ID); err != nil {
		return nil, err
	}
	ledger, err := taskstate.RestoreLedger(state)
	if err != nil {
		return nil, fmt.Errorf("restore normalized task ledger %q: %w", header.ID, err)
	}
	return ledger, nil
}

func loadTaskLedgerNodes(ctx context.Context, tx pgx.Tx, ledgerID taskstate.LedgerID) ([]taskstate.Node, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, parent_id, objective_id, kind, inline_execution, title, status, priority, created_by,
		       assigned_step_id, created_step_id, completed_step_id, acceptance_criteria,
		       metadata, status_reason, created_version, updated_version
		FROM task_nodes WHERE ledger_id=$1 ORDER BY id ASC
		LIMIT $2
	`, ledgerID, maxTaskLedgerNodes+1)
	if err != nil {
		return nil, fmt.Errorf("read task ledger %q nodes: %w", ledgerID, err)
	}
	defer rows.Close()
	nodes := make([]taskstate.Node, 0)
	for rows.Next() {
		var node taskstate.Node
		var parentID, objectiveID *string
		var kind, status, createdBy string
		var criteriaRaw, metadataRaw []byte
		var createdVersion, updatedVersion int64
		if err := rows.Scan(
			&node.ID, &parentID, &objectiveID, &kind, &node.InlineExecution, &node.Title, &status, &node.Priority, &createdBy,
			&node.AssignedStepID, &node.CreatedStepID, &node.CompletedStepID, &criteriaRaw,
			&metadataRaw, &node.StatusReason, &createdVersion, &updatedVersion,
		); err != nil {
			return nil, fmt.Errorf("scan task ledger %q node: %w", ledgerID, err)
		}
		node.ParentID, node.ObjectiveID = taskNodeID(parentID), taskNodeID(objectiveID)
		node.Kind, node.Status, node.CreatedBy = taskstate.NodeKind(kind), taskstate.NodeStatus(status), taskstate.Authority(createdBy)
		if err := json.Unmarshal(criteriaRaw, &node.AcceptanceCriteria); err != nil {
			return nil, fmt.Errorf("decode task ledger %q node %q criteria: %w", ledgerID, node.ID, err)
		}
		if node.Metadata, err = taskstate.NewJSONObject(metadataRaw); err != nil {
			return nil, fmt.Errorf("decode task ledger %q node %q metadata: %w", ledgerID, node.ID, err)
		}
		if node.CreatedVersion, err = taskLedgerVersion(createdVersion); err != nil {
			return nil, fmt.Errorf("task ledger %q node %q created version: %w", ledgerID, node.ID, err)
		}
		if node.UpdatedVersion, err = taskLedgerVersion(updatedVersion); err != nil {
			return nil, fmt.Errorf("task ledger %q node %q updated version: %w", ledgerID, node.ID, err)
		}
		nodes = append(nodes, node)
		if len(nodes) > maxTaskLedgerNodes {
			return nil, fmt.Errorf("%w: task ledger %q exceeds the %d-node limit", taskstate.ErrInvalidState, ledgerID, maxTaskLedgerNodes)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task ledger %q nodes: %w", ledgerID, err)
	}
	rows.Close()
	if err := loadTaskLedgerNodeVerificationRefs(ctx, tx, ledgerID, nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

func taskLedgerVersion(value int64) (uint64, error) {
	if value <= 0 {
		return 0, fmt.Errorf("version must be positive, received %d", value)
	}
	return uint64(value), nil
}

func taskNodeID(value *string) taskstate.NodeID {
	if value == nil {
		return ""
	}
	return taskstate.NodeID(*value)
}

func exactOptionalText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
