package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func persistTaskLedgerMutation(
	ctx context.Context,
	tx pgx.Tx,
	ledgerID taskstate.LedgerID,
	jobID int64,
	jobGeneration int64,
	event taskstate.Event,
	state taskstate.MaterializedState,
) error {
	if err := validateTaskEventShape(event, nil); err != nil {
		return err
	}
	switch event.Kind {
	case taskstate.EventNodeAdded:
		if event.Node == nil {
			return fmt.Errorf("%w: node-added event has no node", taskstate.ErrInvalidState)
		}
		node, err := taskLedgerStateNode(state, event.Node.ID)
		if err != nil {
			return err
		}
		return insertTaskLedgerNode(ctx, tx, ledgerID, jobID, node)
	case taskstate.EventEdgeAdded:
		if event.Edge == nil {
			return fmt.Errorf("%w: edge-added event has no edge", taskstate.ErrInvalidState)
		}
		edge, err := taskLedgerStateEdge(state, event.Edge.ID)
		if err != nil {
			return err
		}
		return insertTaskLedgerEdge(ctx, tx, ledgerID, jobID, edge)
	case taskstate.EventEntryAdded:
		if event.Entry == nil {
			return fmt.Errorf("%w: entry-added event has no entry", taskstate.ErrInvalidState)
		}
		entry, err := taskLedgerStateEntry(state, event.Entry.ID)
		if err != nil {
			return err
		}
		return insertTaskLedgerEntry(ctx, tx, ledgerID, jobID, entry)
	case taskstate.EventEntryRejected, taskstate.EventEntryResolved:
		entry, err := taskLedgerStateEntry(state, event.EntryID)
		if err != nil {
			return err
		}
		if err := updateTaskLedgerEntry(ctx, tx, ledgerID, jobID, entry); err != nil {
			return err
		}
		position := len(entry.Refs) - len(event.VerificationRefs)
		if position < 0 {
			return fmt.Errorf("%w: disposed entry reference projection is inconsistent", taskstate.ErrInvalidState)
		}
		return insertTaskLedgerEntryRefs(ctx, tx, ledgerID, jobID, entry.ID, event.VerificationRefs, position)
	case taskstate.EventEntrySuperseded:
		oldEntry, err := taskLedgerStateEntry(state, event.EntryID)
		if err != nil {
			return err
		}
		replacement, err := taskLedgerStateEntry(state, event.ReplacementID)
		if err != nil {
			return err
		}
		if err := updateTaskLedgerEntry(ctx, tx, ledgerID, jobID, oldEntry); err != nil {
			return err
		}
		return updateTaskLedgerEntry(ctx, tx, ledgerID, jobID, replacement)
	case taskstate.EventDecisionAccepted:
		candidate, err := taskLedgerStateEntry(state, event.EntryID)
		if err != nil {
			return err
		}
		accepted, err := taskLedgerStateEntry(state, event.ReplacementID)
		if err != nil {
			return err
		}
		if err := updateTaskLedgerEntry(ctx, tx, ledgerID, jobID, candidate); err != nil {
			return err
		}
		return insertTaskLedgerEntry(ctx, tx, ledgerID, jobID, accepted)
	case taskstate.EventNodesReadied:
		for _, nodeID := range event.NodeIDs {
			node, err := taskLedgerStateNode(state, nodeID)
			if err != nil {
				return err
			}
			if err := updateTaskLedgerNode(ctx, tx, ledgerID, jobID, node); err != nil {
				return err
			}
		}
		return nil
	case taskstate.EventNodeStepAssigned:
		node, err := taskLedgerStateNode(state, event.NodeID)
		if err != nil {
			return err
		}
		return updateTaskLedgerNode(ctx, tx, ledgerID, jobID, node)
	case taskstate.EventNodeTransitioned:
		node, err := taskLedgerStateNode(state, event.NodeID)
		if err != nil {
			return err
		}
		if err := updateTaskLedgerNode(ctx, tx, ledgerID, jobID, node); err != nil {
			return err
		}
		if node.Status == taskstate.NodeDone ||
			node.Status == taskstate.NodeFailed && len(node.VerificationRefs) == 1 {
			return insertTaskLedgerNodeVerificationRefs(ctx, tx, ledgerID, jobID, node)
		}
		return nil
	case taskstate.EventNodeGenerationSuperseded:
		for _, nodeID := range event.NodeIDs {
			node, err := taskLedgerStateNode(state, nodeID)
			if err != nil {
				return err
			}
			if node.UpdatedVersion == event.Version {
				if err := updateTaskLedgerNode(ctx, tx, ledgerID, jobID, node); err != nil {
					return err
				}
			}
			value, err := taskLedgerStateNodeSupersession(state, nodeID)
			if err != nil {
				return err
			}
			if value.RetiringGeneration != event.RetiringGeneration ||
				value.SupersededAtGeneration != event.SupersededAtGeneration ||
				value.CreatedVersion != event.Version || value.Reason != event.Reason {
				return fmt.Errorf("%w: node %q supersession projection is inconsistent", taskstate.ErrInvalidState, nodeID)
			}
			if err := insertTaskLedgerNodeSupersession(
				ctx, tx, ledgerID, jobID, jobGeneration, value,
			); err != nil {
				return err
			}
		}
		return nil
	case taskstate.EventLedgerClosed:
		return nil
	default:
		return fmt.Errorf("%w: task event kind %q has no persistence mutation", taskstate.ErrInvalidState, event.Kind)
	}
}

func insertTaskLedgerNode(
	ctx context.Context, tx pgx.Tx, ledgerID taskstate.LedgerID, jobID int64, node taskstate.Node,
) error {
	criteria, err := json.Marshal(node.AcceptanceCriteria)
	if err != nil {
		return fmt.Errorf("encode task node %q criteria: %w", node.ID, err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO task_nodes (
			ledger_id, job_id, id, parent_id, objective_id, kind, title, status, priority,
			created_by, assigned_step_id, created_step_id, completed_step_id,
			acceptance_criteria, metadata, status_reason, created_version, updated_version
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
			$14::jsonb, $15::jsonb, $16, $17, $18
		)
	`, ledgerID, jobID, node.ID, nullableTaskText(string(node.ParentID)), nullableTaskText(string(node.ObjectiveID)),
		node.Kind, node.Title, node.Status, node.Priority, node.CreatedBy, node.AssignedStepID,
		node.CreatedStepID, node.CompletedStepID, criteria, node.Metadata.Bytes(), node.StatusReason,
		int64(node.CreatedVersion), int64(node.UpdatedVersion))
	if err != nil {
		return fmt.Errorf("insert task ledger %q node %q: %w", ledgerID, node.ID, err)
	}
	return nil
}

func updateTaskLedgerNode(
	ctx context.Context, tx pgx.Tx, ledgerID taskstate.LedgerID, jobID int64, node taskstate.Node,
) error {
	result, err := tx.Exec(ctx, `
		UPDATE task_nodes
		SET status=$4, assigned_step_id=$5, completed_step_id=$6,
		    status_reason=$7, updated_version=$8, updated_at=NOW()
		WHERE ledger_id=$1 AND job_id=$2 AND id=$3
		  AND created_version=$9 AND updated_version < $8
	`, ledgerID, jobID, node.ID, node.Status, node.AssignedStepID, node.CompletedStepID,
		node.StatusReason, int64(node.UpdatedVersion), int64(node.CreatedVersion))
	return requireTaskLedgerRow(result, err, fmt.Sprintf("update task ledger %q node %q", ledgerID, node.ID))
}

func insertTaskLedgerEdge(
	ctx context.Context, tx pgx.Tx, ledgerID taskstate.LedgerID, jobID int64, edge taskstate.Edge,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO task_node_edges (
			ledger_id, job_id, id, from_node_id, to_node_id, kind, created_version
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, ledgerID, jobID, edge.ID, edge.From, edge.To, edge.Kind, int64(edge.CreatedVersion))
	if err != nil {
		return fmt.Errorf("insert task ledger %q edge %q: %w", ledgerID, edge.ID, err)
	}
	return nil
}

func insertTaskLedgerEntry(
	ctx context.Context, tx pgx.Tx, ledgerID taskstate.LedgerID, jobID int64, entry taskstate.Entry,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO task_entries (
			ledger_id, job_id, id, scope_node_id, kind, feedback_purpose, status, authority,
			content, content_sha256, confidence, created_by, created_step_id, supersedes_id,
			source_entry_id, acceptance_policy, accepted_by, metadata,
			disposition_reason, disposition_by, created_version, updated_version
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18::jsonb, $19, $20, $21, $22
		)
	`, ledgerID, jobID, entry.ID, nullableTaskText(string(entry.ScopeNodeID)), entry.Kind,
		nullableTaskText(string(entry.FeedbackPurpose)), entry.Status, entry.Authority, entry.Content,
		entry.ContentSHA256, entry.Confidence, entry.CreatedBy, entry.CreatedStepID,
		nullableTaskText(string(entry.SupersedesID)), nullableTaskText(string(entry.Provenance.SourceEntryID)),
		nullableTaskText(entry.Provenance.AcceptancePolicy), nullableTaskText(string(entry.Provenance.AcceptedBy)),
		entry.Metadata.Bytes(), entry.DispositionReason, nullableTaskText(string(entry.DispositionBy)),
		int64(entry.CreatedVersion), int64(entry.UpdatedVersion))
	if err != nil {
		return fmt.Errorf("insert task ledger %q entry %q: %w", ledgerID, entry.ID, err)
	}
	return insertTaskLedgerEntryRefs(ctx, tx, ledgerID, jobID, entry.ID, entry.Refs, 0)
}

func updateTaskLedgerEntry(
	ctx context.Context, tx pgx.Tx, ledgerID taskstate.LedgerID, jobID int64, entry taskstate.Entry,
) error {
	result, err := tx.Exec(ctx, `
		UPDATE task_entries
		SET status=$4, supersedes_id=$5, disposition_reason=$6, disposition_by=$7,
		    updated_version=$8, updated_at=NOW()
		WHERE ledger_id=$1 AND job_id=$2 AND id=$3
		  AND created_version=$9 AND updated_version < $8
	`, ledgerID, jobID, entry.ID, entry.Status, nullableTaskText(string(entry.SupersedesID)),
		entry.DispositionReason, nullableTaskText(string(entry.DispositionBy)),
		int64(entry.UpdatedVersion), int64(entry.CreatedVersion))
	return requireTaskLedgerRow(result, err, fmt.Sprintf("update task ledger %q entry %q", ledgerID, entry.ID))
}

func insertTaskLedgerEntryRefs(
	ctx context.Context, tx pgx.Tx, ledgerID taskstate.LedgerID, jobID int64,
	entryID taskstate.EntryID, refs []taskstate.Ref, startPosition int,
) error {
	for index, ref := range refs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO task_entry_refs (
				ledger_id, job_id, entry_id, uri, version, content_sha256, relation, position
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, ledgerID, jobID, entryID, ref.URI, ref.Version, ref.Hash, ref.Relation, startPosition+index); err != nil {
			return fmt.Errorf("insert task ledger %q entry %q reference: %w", ledgerID, entryID, err)
		}
	}
	return nil
}
