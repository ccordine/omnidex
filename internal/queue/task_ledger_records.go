package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func loadTaskLedgerEdges(ctx context.Context, tx pgx.Tx, ledgerID taskstate.LedgerID) ([]taskstate.Edge, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, kind, from_node_id, to_node_id, created_version
		FROM task_node_edges WHERE ledger_id=$1 ORDER BY id ASC
		LIMIT $2
	`, ledgerID, maxTaskLedgerEdges+1)
	if err != nil {
		return nil, fmt.Errorf("read task ledger %q edges: %w", ledgerID, err)
	}
	defer rows.Close()
	edges := make([]taskstate.Edge, 0)
	for rows.Next() {
		var edge taskstate.Edge
		var kind string
		var createdVersion int64
		if err := rows.Scan(&edge.ID, &kind, &edge.From, &edge.To, &createdVersion); err != nil {
			return nil, fmt.Errorf("scan task ledger %q edge: %w", ledgerID, err)
		}
		edge.Kind = taskstate.EdgeKind(kind)
		if edge.CreatedVersion, err = taskLedgerVersion(createdVersion); err != nil {
			return nil, fmt.Errorf("task ledger %q edge %q created version: %w", ledgerID, edge.ID, err)
		}
		edges = append(edges, edge)
		if len(edges) > maxTaskLedgerEdges {
			return nil, fmt.Errorf("%w: task ledger %q exceeds the %d-edge limit", taskstate.ErrInvalidState, ledgerID, maxTaskLedgerEdges)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task ledger %q edges: %w", ledgerID, err)
	}
	return edges, nil
}

func loadTaskLedgerEntries(ctx context.Context, tx pgx.Tx, ledgerID taskstate.LedgerID) ([]taskstate.Entry, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, scope_node_id, kind, feedback_purpose, status, authority, content,
		       content_sha256, confidence, created_by, created_step_id, supersedes_id,
		       source_entry_id, acceptance_policy, accepted_by, metadata,
		       disposition_reason, disposition_by, created_version, updated_version
		FROM task_entries WHERE ledger_id=$1 ORDER BY id ASC
		LIMIT $2
	`, ledgerID, maxTaskLedgerEntries+1)
	if err != nil {
		return nil, fmt.Errorf("read task ledger %q entries: %w", ledgerID, err)
	}
	defer rows.Close()
	entries := make([]taskstate.Entry, 0)
	for rows.Next() {
		var entry taskstate.Entry
		var scopeNodeID, feedbackPurpose, supersedesID *string
		var sourceEntryID, acceptancePolicy, acceptedBy *string
		var kind, status, authority, createdBy string
		var dispositionBy *string
		var metadataRaw []byte
		var createdVersion, updatedVersion int64
		if err := rows.Scan(
			&entry.ID, &scopeNodeID, &kind, &feedbackPurpose, &status, &authority,
			&entry.Content, &entry.ContentSHA256, &entry.Confidence, &createdBy,
			&entry.CreatedStepID, &supersedesID, &sourceEntryID, &acceptancePolicy,
			&acceptedBy, &metadataRaw,
			&entry.DispositionReason, &dispositionBy, &createdVersion, &updatedVersion,
		); err != nil {
			return nil, fmt.Errorf("scan task ledger %q entry: %w", ledgerID, err)
		}
		entry.ScopeNodeID = taskstate.NodeID(exactOptionalText(scopeNodeID))
		entry.Kind, entry.FeedbackPurpose = taskstate.EntryKind(kind), taskstate.FeedbackPurpose(exactOptionalText(feedbackPurpose))
		entry.Status, entry.Authority = taskstate.EntryStatus(status), taskstate.Authority(authority)
		entry.CreatedBy, entry.SupersedesID = taskstate.Authority(createdBy), taskstate.EntryID(exactOptionalText(supersedesID))
		entry.DispositionBy = taskstate.Authority(exactOptionalText(dispositionBy))
		entry.Provenance = taskstate.EntryProvenance{
			SourceEntryID:    taskstate.EntryID(exactOptionalText(sourceEntryID)),
			AcceptancePolicy: exactOptionalText(acceptancePolicy),
			AcceptedBy:       taskstate.Authority(exactOptionalText(acceptedBy)),
		}
		entry.Refs = make([]taskstate.Ref, 0)
		if entry.Metadata, err = taskstate.NewJSONObject(metadataRaw); err != nil {
			return nil, fmt.Errorf("decode task ledger %q entry %q metadata: %w", ledgerID, entry.ID, err)
		}
		if entry.CreatedVersion, err = taskLedgerVersion(createdVersion); err != nil {
			return nil, fmt.Errorf("task ledger %q entry %q created version: %w", ledgerID, entry.ID, err)
		}
		if entry.UpdatedVersion, err = taskLedgerVersion(updatedVersion); err != nil {
			return nil, fmt.Errorf("task ledger %q entry %q updated version: %w", ledgerID, entry.ID, err)
		}
		entries = append(entries, entry)
		if len(entries) > maxTaskLedgerEntries {
			return nil, fmt.Errorf("%w: task ledger %q exceeds the %d-entry limit", taskstate.ErrInvalidState, ledgerID, maxTaskLedgerEntries)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task ledger %q entries: %w", ledgerID, err)
	}
	if err := loadTaskLedgerEntryRefs(ctx, tx, ledgerID, entries); err != nil {
		return nil, err
	}
	if err := deriveTaskLedgerSupersession(entries); err != nil {
		return nil, fmt.Errorf("task ledger %q supersession state: %w", ledgerID, err)
	}
	return entries, nil
}

func loadTaskLedgerEntryRefs(
	ctx context.Context,
	tx pgx.Tx,
	ledgerID taskstate.LedgerID,
	entries []taskstate.Entry,
) error {
	index := make(map[taskstate.EntryID]int, len(entries))
	for entryIndex := range entries {
		index[entries[entryIndex].ID] = entryIndex
	}
	rows, err := tx.Query(ctx, `
		SELECT entry_id, uri, version, content_sha256, relation, position
		FROM task_entry_refs
		WHERE ledger_id=$1
		ORDER BY entry_id, position
		LIMIT $2
	`, ledgerID, maxTaskLedgerRefs+1)
	if err != nil {
		return fmt.Errorf("read task ledger %q entry references: %w", ledgerID, err)
	}
	defer rows.Close()
	refCount := 0
	for rows.Next() {
		refCount++
		if refCount > maxTaskLedgerRefs {
			return fmt.Errorf("%w: task ledger %q exceeds the %d-reference limit", taskstate.ErrInvalidState, ledgerID, maxTaskLedgerRefs)
		}
		var entryID taskstate.EntryID
		var ref taskstate.Ref
		var relation string
		var position int
		if err := rows.Scan(&entryID, &ref.URI, &ref.Version, &ref.Hash, &relation, &position); err != nil {
			return fmt.Errorf("scan task ledger %q entry reference: %w", ledgerID, err)
		}
		entryIndex, ok := index[entryID]
		if !ok {
			return fmt.Errorf("%w: task ledger %q reference targets missing entry %q", taskstate.ErrInvalidState, ledgerID, entryID)
		}
		if position != len(entries[entryIndex].Refs) {
			return fmt.Errorf("%w: task ledger %q entry %q reference positions are not contiguous", taskstate.ErrInvalidState, ledgerID, entryID)
		}
		ref.Relation = taskstate.RefRelation(relation)
		entries[entryIndex].Refs = append(entries[entryIndex].Refs, ref)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate task ledger %q entry references: %w", ledgerID, err)
	}
	return nil
}

func deriveTaskLedgerSupersession(entries []taskstate.Entry) error {
	index := make(map[taskstate.EntryID]int, len(entries))
	for entryIndex := range entries {
		if _, exists := index[entries[entryIndex].ID]; exists {
			return fmt.Errorf("duplicate entry %q", entries[entryIndex].ID)
		}
		index[entries[entryIndex].ID] = entryIndex
	}
	for entryIndex := range entries {
		oldID := entries[entryIndex].SupersedesID
		if oldID != "" {
			if err := assignTaskLedgerReplacement(entries, index, oldID, entries[entryIndex].ID); err != nil {
				return err
			}
		}
		sourceID := entries[entryIndex].Provenance.SourceEntryID
		if sourceID != "" {
			if err := assignTaskLedgerReplacement(entries, index, sourceID, entries[entryIndex].ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func assignTaskLedgerReplacement(
	entries []taskstate.Entry,
	index map[taskstate.EntryID]int,
	oldID taskstate.EntryID,
	replacementID taskstate.EntryID,
) error {
	oldIndex, exists := index[oldID]
	if !exists {
		return fmt.Errorf("entry %q replaces missing entry %q", replacementID, oldID)
	}
	if entries[oldIndex].SupersededBy != "" {
		return fmt.Errorf("entry %q has multiple replacements", oldID)
	}
	entries[oldIndex].SupersededBy = replacementID
	return nil
}
