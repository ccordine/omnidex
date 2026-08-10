package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
)

func loadWorkingSetSnapshotTx(
	ctx context.Context,
	tx pgx.Tx,
	ledger taskLedgerHeader,
	generation int64,
	lock bool,
) (workingset.Snapshot, error) {
	query := `
		SELECT id, ledger_id, scope_kind, scope_id,
		       max_items, max_bytes, max_pinned_items, max_pinned_bytes,
		       status, version, clock, closed_tick, close_reason
		FROM working_sets
		WHERE job_id=$1 AND generation=$2
	`
	if lock {
		query += " FOR UPDATE"
	}
	var snapshot workingset.Snapshot
	var ledgerID, scopeKind, status string
	var version, clock, closedTick int64
	if err := tx.QueryRow(ctx, query, ledger.Owner.JobID, generation).Scan(
		&snapshot.ID, &ledgerID, &scopeKind, &snapshot.Scope.ID,
		&snapshot.Budget.MaxItems, &snapshot.Budget.MaxBytes,
		&snapshot.Budget.MaxPinnedItems, &snapshot.Budget.MaxPinnedBytes,
		&status, &version, &clock, &closedTick, &snapshot.CloseReason,
	); isWorkingSetNotFound(err) {
		return workingset.Snapshot{}, workingSetNotFound(ledger.Owner.JobID, generation)
	} else if err != nil {
		return workingset.Snapshot{}, fmt.Errorf("read working set for job %d generation %d: %w", ledger.Owner.JobID, generation, err)
	}
	if ledgerID != string(ledger.ID) {
		return workingset.Snapshot{}, fmt.Errorf(
			"%w: working set %q is bound to ledger %q, expected %q",
			workingset.ErrInvalidSet, snapshot.ID, ledgerID, ledger.ID,
		)
	}
	snapshot.Schema = workingset.WorkingSetSchemaV1
	snapshot.Owner = workingset.Owner{
		LedgerID: ledger.ID, JobID: ledger.Owner.JobID, Generation: generation,
	}
	snapshot.Scope.Kind = workingset.ScopeKind(scopeKind)
	snapshot.Status = workingset.Status(status)
	if snapshot.Version, snapshot.Clock, snapshot.ClosedTick = uint64(version), uint64(clock), uint64(closedTick); version < 0 || clock < 0 || closedTick < 0 {
		return workingset.Snapshot{}, fmt.Errorf("%w: working set %q contains a negative counter", workingset.ErrInvalidSet, snapshot.ID)
	}
	items, err := loadWorkingSetItemsTx(ctx, tx, snapshot.ID, lock)
	if err != nil {
		return workingset.Snapshot{}, err
	}
	snapshot.Items = items
	if err := loadWorkingSetMembershipsTx(
		ctx, tx, snapshot.ID, snapshot.Version, lock, snapshot.Items,
	); err != nil {
		return workingset.Snapshot{}, err
	}
	closed, err := loadWorkingSetClosedScopesTx(ctx, tx, snapshot.ID, snapshot.Clock, lock)
	if err != nil {
		return workingset.Snapshot{}, err
	}
	snapshot.ClosedScopes = closed
	set, err := workingset.Restore(snapshot)
	if err != nil {
		return workingset.Snapshot{}, fmt.Errorf("restore normalized working set %q: %w", snapshot.ID, err)
	}
	return set.Snapshot(), nil
}

func loadWorkingSetItemsTx(
	ctx context.Context,
	tx pgx.Tx,
	setID workingset.SetID,
	lock bool,
) ([]workingset.Item, error) {
	query := `
		SELECT item_id, ref_uri, ref_version, ref_sha256, ref_relation,
		       role, retention, priority, state, byte_cost,
		       provider, operation_id, acquisition_reason,
		       use_count, reacquisition_count, created_tick, last_used_tick, released_tick, disposition_reason
		FROM working_set_items WHERE working_set_id=$1
		ORDER BY item_id ASC LIMIT $2
	`
	if lock {
		query += " FOR UPDATE"
	}
	rows, err := tx.Query(ctx, query, setID, workingset.MaxHistoricalItems+1)
	if err != nil {
		return nil, fmt.Errorf("read working set %q items: %w", setID, err)
	}
	defer rows.Close()
	items := make([]workingset.Item, 0)
	for rows.Next() {
		var item workingset.Item
		var relation, role, retention, state, provider string
		var useCount, reacquisitionCount, createdTick, lastUsedTick, releasedTick int64
		if err := rows.Scan(
			&item.ID, &item.Ref.URI, &item.Ref.Version, &item.Ref.Hash, &relation,
			&role, &retention, &item.Priority, &state, &item.ByteCost,
			&provider, &item.Acquisition.OperationID, &item.Acquisition.Reason,
			&useCount, &reacquisitionCount, &createdTick, &lastUsedTick, &releasedTick, &item.DispositionReason,
		); err != nil {
			return nil, fmt.Errorf("scan working set %q item: %w", setID, err)
		}
		if useCount < 0 || reacquisitionCount < 0 || createdTick < 0 || lastUsedTick < 0 || releasedTick < 0 {
			return nil, fmt.Errorf("%w: working set %q item %q contains a negative counter", workingset.ErrInvalidSet, setID, item.ID)
		}
		item.Ref.Relation = taskstate.RefRelation(relation)
		item.Role, item.Retention = workingset.Role(role), workingset.Retention(retention)
		item.State, item.Acquisition.Provider = workingset.ItemState(state), workingset.Provider(provider)
		item.UseCount, item.CreatedTick = uint64(useCount), uint64(createdTick)
		item.ReacquisitionCount = uint64(reacquisitionCount)
		item.LastUsedTick, item.ReleasedTick = uint64(lastUsedTick), uint64(releasedTick)
		item.Memberships = make([]workingset.Membership, 0)
		items = append(items, item)
		if len(items) > workingset.MaxHistoricalItems {
			return nil, fmt.Errorf("%w: working set %q exceeds the historical item limit", workingset.ErrCapacityExceeded, setID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate working set %q items: %w", setID, err)
	}
	return items, nil
}

func loadWorkingSetMembershipsTx(
	ctx context.Context,
	tx pgx.Tx,
	setID workingset.SetID,
	setVersion uint64,
	lock bool,
	items []workingset.Item,
) error {
	query := `
		SELECT item_id, scope_kind, scope_id, retention, created_version, updated_version
		FROM working_set_memberships WHERE working_set_id=$1
		ORDER BY item_id ASC,
		 CASE retention WHEN 'call' THEN 1 WHEN 'step' THEN 2 WHEN 'phase' THEN 3
		  WHEN 'task' THEN 4 WHEN 'objective' THEN 5 WHEN 'job' THEN 6 ELSE 7 END,
		 scope_kind ASC, scope_id ASC LIMIT $2
	`
	if lock {
		query += " FOR UPDATE"
	}
	rows, err := tx.Query(ctx, query, setID, workingset.MaxMemberships+1)
	if err != nil {
		return fmt.Errorf("read working set %q memberships: %w", setID, err)
	}
	defer rows.Close()
	index := make(map[workingset.ItemID]int, len(items))
	for position := range items {
		index[items[position].ID] = position
	}
	count := 0
	for rows.Next() {
		var itemID workingset.ItemID
		var membership workingset.Membership
		var createdVersion, updatedVersion int64
		if err := rows.Scan(
			&itemID, &membership.Scope.Kind, &membership.Scope.ID, &membership.Retention,
			&createdVersion, &updatedVersion,
		); err != nil {
			return fmt.Errorf("scan working set %q membership: %w", setID, err)
		}
		if createdVersion <= 0 || updatedVersion < createdVersion || uint64(updatedVersion) > setVersion {
			return fmt.Errorf(
				"%w: working set %q membership for %q has an invalid version",
				workingset.ErrInvalidSet, setID, itemID,
			)
		}
		position, exists := index[itemID]
		if !exists {
			return fmt.Errorf("%w: working set %q membership references unknown item %q", workingset.ErrInvalidSet, setID, itemID)
		}
		items[position].Memberships = append(items[position].Memberships, membership)
		count++
		if count > workingset.MaxMemberships {
			return fmt.Errorf("%w: working set %q exceeds the membership limit", workingset.ErrCapacityExceeded, setID)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate working set %q memberships: %w", setID, err)
	}
	return nil
}

func loadWorkingSetClosedScopesTx(
	ctx context.Context,
	tx pgx.Tx,
	setID workingset.SetID,
	setClock uint64,
	lock bool,
) ([]workingset.Scope, error) {
	query := `
		SELECT scope_kind, scope_id, closed_tick FROM working_set_closed_scopes
		WHERE working_set_id=$1 ORDER BY scope_kind ASC, scope_id ASC LIMIT $2
	`
	if lock {
		query += " FOR UPDATE"
	}
	rows, err := tx.Query(ctx, query, setID, workingset.MaxClosedScopes+1)
	if err != nil {
		return nil, fmt.Errorf("read working set %q closed scopes: %w", setID, err)
	}
	defer rows.Close()
	scopes := make([]workingset.Scope, 0)
	for rows.Next() {
		var scope workingset.Scope
		var closedTick int64
		if err := rows.Scan(&scope.Kind, &scope.ID, &closedTick); err != nil {
			return nil, fmt.Errorf("scan working set %q closed scope: %w", setID, err)
		}
		if closedTick <= 0 || uint64(closedTick) > setClock {
			return nil, fmt.Errorf(
				"%w: working set %q closed scope has an invalid tick", workingset.ErrInvalidSet, setID,
			)
		}
		scopes = append(scopes, scope)
		if len(scopes) > workingset.MaxClosedScopes {
			return nil, fmt.Errorf("%w: working set %q exceeds the closed-scope limit", workingset.ErrCapacityExceeded, setID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate working set %q closed scopes: %w", setID, err)
	}
	return scopes, nil
}
