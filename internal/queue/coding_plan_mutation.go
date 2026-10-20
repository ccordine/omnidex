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

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ApplyCodingLedgerMutation(ctx context.Context, command ApplyCodingLedgerMutationCommand) (CodingLedgerRevision, error) {
	return r.applyCodingLedgerMutation(ctx, command, "planning")
}

// ApplyCodingLedgerControl persists a deterministic owner control without
// misrepresenting it as semantic planning input for a later model audit.
func (r *Repository) ApplyCodingLedgerControl(ctx context.Context, command ApplyCodingLedgerMutationCommand) (CodingLedgerRevision, error) {
	return r.applyCodingLedgerMutation(ctx, command, "planning_control")
}

func (r *Repository) applyCodingLedgerMutation(ctx context.Context, command ApplyCodingLedgerMutationCommand, ownerAction string) (CodingLedgerRevision, error) {
	if r == nil || r.pool == nil || command.LedgerID <= 0 || strings.TrimSpace(command.OwnerTurnText) == "" {
		return CodingLedgerRevision{}, fmt.Errorf("coding ledger mutation requires repository, ledger, and exact owner turn")
	}
	if ownerAction != "planning" && ownerAction != "planning_control" {
		return CodingLedgerRevision{}, fmt.Errorf("coding ledger mutation requires an exact owner action")
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

	result, err := applyCodingLedgerMutationTx(ctx, tx, command, ownerAction)
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	return result, tx.Commit(ctx)
}

func applyCodingLedgerMutationTx(ctx context.Context, tx pgx.Tx, command ApplyCodingLedgerMutationCommand, ownerAction string) (CodingLedgerRevision, error) {
	var current int64
	var state string
	if err := tx.QueryRow(ctx, `SELECT current_revision,state FROM coding_ledgers WHERE id=$1 FOR UPDATE`, command.LedgerID).Scan(&current, &state); err != nil {
		return CodingLedgerRevision{}, err
	}
	var replayRevision, replayBase int64
	var replayOwner, replayAction, replayText string
	var replayMutation []byte
	err := tx.QueryRow(ctx, `
		SELECT revision.revision,revision.base_revision,revision.owner_turn_id,turn_record.action,turn_record.exact_text,
		       revision.mutation_payload
		FROM coding_ledger_revisions revision
		JOIN coding_ledger_owner_turns turn_record
		  ON turn_record.ledger_id=revision.ledger_id AND turn_record.owner_turn_id=revision.owner_turn_id
		WHERE revision.ledger_id=$1 AND revision.operation_id=$2
	`, command.LedgerID, command.Mutation.OperationID).Scan(
		&replayRevision, &replayBase, &replayOwner, &replayAction, &replayText, &replayMutation,
	)
	if err == nil {
		expectedMutation, encodeErr := json.Marshal(command.Mutation)
		if encodeErr != nil {
			return CodingLedgerRevision{}, encodeErr
		}
		if replayBase != command.Mutation.BaseRevision || replayOwner != command.Mutation.OwnerTurnID ||
			replayAction != ownerAction ||
			replayText != command.OwnerTurnText ||
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
		current, ownerAction, command.OwnerTurnText); err != nil {
		return CodingLedgerRevision{}, err
	}
	mutationPayload, err := json.Marshal(command.Mutation)
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO coding_ledger_revisions(
			ledger_id,revision,base_revision,operation_id,owner_turn_id,state,mutation_payload
		) VALUES($1,$2,$3,$4,$5,'proposed',$6::jsonb)
	`, command.LedgerID, next.Revision, current, command.Mutation.OperationID,
		command.Mutation.OwnerTurnID, string(mutationPayload)); err != nil {
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
	}
	persistedSet := make(map[model.LedgerItemID]bool)
	for _, id := range expectedIDs {
		persistedSet[id] = true
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

func resetCodingLedgerAffectedAgreements(ledger *model.EngineeringLedger, id model.LedgerItemID) {
	setCodingLedgerItem(ledger, id, func(item *model.LedgerItem) {
		if item.Disposition == model.LedgerDispositionActive {
			item.ReviewState = model.LedgerReviewProposed
		}
	})
}

func ledgerMutationItemID(item *model.LedgerMutationItem) model.LedgerItemID {
	if item != nil && item.Task != nil {
		return item.Task.ID
	}
	return ""
}

func ledgerHasItem(ledger model.EngineeringLedger, id model.LedgerItemID) bool {
	found := false
	setCodingLedgerItem(&ledger, id, func(*model.LedgerItem) { found = true })
	return found
}

func appendCodingLedgerMutationItem(ledger *model.EngineeringLedger, item *model.LedgerMutationItem) {
	if item != nil && item.Task != nil {
		ledger.Tasks = append(ledger.Tasks, *item.Task)
	}
}

func replaceCodingLedgerMutationItem(ledger *model.EngineeringLedger, id model.LedgerItemID, item *model.LedgerMutationItem) bool {
	if item == nil || item.Task == nil {
		return false
	}
	for i := range ledger.Tasks {
		if ledger.Tasks[i].ID == id {
			ledger.Tasks[i] = *item.Task
			return true
		}
	}
	return false
}

func setCodingLedgerItem(ledger *model.EngineeringLedger, id model.LedgerItemID, update func(*model.LedgerItem)) bool {
	for i := range ledger.Tasks {
		if ledger.Tasks[i].ID == id {
			update(&ledger.Tasks[i].LedgerItem)
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
