package queue

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5"
)

func persistWorkingSetDiffTx(
	ctx context.Context,
	tx pgx.Tx,
	before, after workingset.Snapshot,
) error {
	if before.ID != after.ID || before.Owner != after.Owner || before.Scope != after.Scope ||
		before.Budget != after.Budget || after.Version != before.Version+1 || after.Clock != after.Version {
		return fmt.Errorf("%w: working-set command changed immutable identity or skipped a version", workingset.ErrInvalidSet)
	}
	beforeItems := workingSetItemMap(before.Items)
	afterItems := workingSetItemMap(after.Items)
	for id := range beforeItems {
		if _, exists := afterItems[id]; !exists {
			return fmt.Errorf("%w: working-set command deleted historical item %q", workingset.ErrInvalidSet, id)
		}
	}
	for _, item := range after.Items {
		prior, exists := beforeItems[item.ID]
		if !exists {
			if err := insertWorkingSetItemTx(ctx, tx, after, item); err != nil {
				return err
			}
		} else if err := updateWorkingSetItemTx(ctx, tx, after, prior, item); err != nil {
			return err
		}
	}
	if err := persistWorkingSetMembershipDiffTx(ctx, tx, before, after); err != nil {
		return err
	}
	if err := persistWorkingSetClosedScopeDiffTx(ctx, tx, before, after); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `
		UPDATE working_sets
		SET status=$4, version=$5, clock=$6, closed_tick=$7, close_reason=$8,
		    closed_at=CASE WHEN $4='closed' THEN NOW() ELSE NULL END, updated_at=NOW()
		WHERE id=$1 AND job_id=$2 AND generation=$3 AND version=$9 AND status=$10
	`, after.ID, after.Owner.JobID, after.Owner.Generation, after.Status,
		int64(after.Version), int64(after.Clock), int64(after.ClosedTick), after.CloseReason,
		int64(before.Version), before.Status)
	if err != nil {
		return fmt.Errorf("advance working set %q: %w", after.ID, err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: working set %q changed during command", workingset.ErrVersionConflict, after.ID)
	}
	return nil
}

func insertWorkingSetItemTx(
	ctx context.Context,
	tx pgx.Tx,
	snapshot workingset.Snapshot,
	item workingset.Item,
) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO working_set_items (
			working_set_id, job_id, generation, item_id,
			ref_uri, ref_version, ref_sha256, ref_relation,
			role, retention, priority, state, byte_cost,
			provider, operation_id, acquisition_reason,
			use_count, created_tick, last_used_tick, released_tick, disposition_reason
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
	`, snapshot.ID, snapshot.Owner.JobID, snapshot.Owner.Generation, item.ID,
		item.Ref.URI, item.Ref.Version, item.Ref.Hash, item.Ref.Relation,
		item.Role, item.Retention, item.Priority, item.State, item.ByteCost,
		item.Acquisition.Provider, item.Acquisition.OperationID, item.Acquisition.Reason,
		int64(item.UseCount), int64(item.CreatedTick), int64(item.LastUsedTick),
		int64(item.ReleasedTick), item.DispositionReason,
	); err != nil {
		return fmt.Errorf("insert working set %q item %q: %w", snapshot.ID, item.ID, err)
	}
	return nil
}

func updateWorkingSetItemTx(
	ctx context.Context,
	tx pgx.Tx,
	snapshot workingset.Snapshot,
	before, after workingset.Item,
) error {
	if before.ID != after.ID || before.Ref != after.Ref || before.Role != after.Role ||
		before.Priority != after.Priority || before.ByteCost != after.ByteCost ||
		before.Acquisition != after.Acquisition || before.CreatedTick != after.CreatedTick {
		return fmt.Errorf("%w: working-set command changed immutable item %q fields", workingset.ErrInvalidSet, before.ID)
	}
	beforeWithoutMemberships, afterWithoutMemberships := before, after
	beforeWithoutMemberships.Memberships, afterWithoutMemberships.Memberships = nil, nil
	if reflect.DeepEqual(beforeWithoutMemberships, afterWithoutMemberships) {
		return nil
	}
	result, err := tx.Exec(ctx, `
		UPDATE working_set_items
		SET retention=$5, state=$6, use_count=$7, last_used_tick=$8,
		    released_tick=$9, disposition_reason=$10, updated_at=NOW()
		WHERE working_set_id=$1 AND job_id=$2 AND generation=$3 AND item_id=$4
	`, snapshot.ID, snapshot.Owner.JobID, snapshot.Owner.Generation, after.ID,
		after.Retention, after.State, int64(after.UseCount), int64(after.LastUsedTick),
		int64(after.ReleasedTick), after.DispositionReason)
	if err != nil {
		return fmt.Errorf("update working set %q item %q: %w", snapshot.ID, after.ID, err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: working set %q item %q disappeared", workingset.ErrInvalidSet, snapshot.ID, after.ID)
	}
	return nil
}

type workingSetMembershipKey struct {
	itemID    workingset.ItemID
	scopeKind workingset.ScopeKind
	scopeID   workingset.ScopeID
}

func persistWorkingSetMembershipDiffTx(
	ctx context.Context,
	tx pgx.Tx,
	before, after workingset.Snapshot,
) error {
	oldMemberships := workingSetMembershipMap(before.Items)
	newMemberships := workingSetMembershipMap(after.Items)
	for key, prior := range oldMemberships {
		current, exists := newMemberships[key]
		if !exists {
			result, err := tx.Exec(ctx, `
				DELETE FROM working_set_memberships
				WHERE working_set_id=$1 AND item_id=$2 AND scope_kind=$3 AND scope_id=$4
			`, after.ID, key.itemID, key.scopeKind, key.scopeID)
			if err != nil || result.RowsAffected() != 1 {
				return workingSetMembershipWriteError("delete", after.ID, key, result.RowsAffected(), err)
			}
		} else if prior.Retention != current.Retention {
			result, err := tx.Exec(ctx, `
				UPDATE working_set_memberships
				SET retention=$5, updated_version=$6, updated_at=NOW()
				WHERE working_set_id=$1 AND item_id=$2 AND scope_kind=$3 AND scope_id=$4
			`, after.ID, key.itemID, key.scopeKind, key.scopeID, current.Retention, int64(after.Version))
			if err != nil || result.RowsAffected() != 1 {
				return workingSetMembershipWriteError("update", after.ID, key, result.RowsAffected(), err)
			}
		}
	}
	for key, membership := range newMemberships {
		if _, exists := oldMemberships[key]; exists {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO working_set_memberships (
				working_set_id, job_id, generation, item_id, scope_kind, scope_id,
				retention, created_version, updated_version
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)
		`, after.ID, after.Owner.JobID, after.Owner.Generation, key.itemID,
			key.scopeKind, key.scopeID, membership.Retention, int64(after.Version)); err != nil {
			return fmt.Errorf("insert working set %q membership for %q: %w", after.ID, key.itemID, err)
		}
	}
	return nil
}

func persistWorkingSetClosedScopeDiffTx(
	ctx context.Context,
	tx pgx.Tx,
	before, after workingset.Snapshot,
) error {
	oldScopes := make(map[string]struct{}, len(before.ClosedScopes))
	for _, scope := range before.ClosedScopes {
		oldScopes[workingSetScopeKey(scope)] = struct{}{}
	}
	for _, scope := range after.ClosedScopes {
		if _, exists := oldScopes[workingSetScopeKey(scope)]; exists {
			delete(oldScopes, workingSetScopeKey(scope))
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO working_set_closed_scopes (
				working_set_id, job_id, generation, scope_kind, scope_id, closed_tick
			) VALUES ($1,$2,$3,$4,$5,$6)
		`, after.ID, after.Owner.JobID, after.Owner.Generation,
			scope.Kind, scope.ID, int64(after.Version)); err != nil {
			return fmt.Errorf("insert working set %q closed scope: %w", after.ID, err)
		}
	}
	if len(oldScopes) != 0 {
		return fmt.Errorf("%w: working-set command reopened a closed scope", workingset.ErrInvalidSet)
	}
	return nil
}

func workingSetItemMap(items []workingset.Item) map[workingset.ItemID]workingset.Item {
	result := make(map[workingset.ItemID]workingset.Item, len(items))
	for _, item := range items {
		result[item.ID] = item
	}
	return result
}

func workingSetMembershipMap(items []workingset.Item) map[workingSetMembershipKey]workingset.Membership {
	result := make(map[workingSetMembershipKey]workingset.Membership)
	for _, item := range items {
		for _, membership := range item.Memberships {
			key := workingSetMembershipKey{
				itemID: item.ID, scopeKind: membership.Scope.Kind, scopeID: membership.Scope.ID,
			}
			result[key] = membership
		}
	}
	return result
}

func workingSetScopeKey(scope workingset.Scope) string {
	return string(scope.Kind) + "\x00" + string(scope.ID)
}

func workingSetMembershipWriteError(
	action string,
	setID workingset.SetID,
	key workingSetMembershipKey,
	rows int64,
	err error,
) error {
	if err != nil {
		return fmt.Errorf("%s working set %q membership for %q: %w", action, setID, key.itemID, err)
	}
	return fmt.Errorf(
		"%w: %s working set %q membership for %q affected %d rows",
		workingset.ErrInvalidSet, action, setID, key.itemID, rows,
	)
}
