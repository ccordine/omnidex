package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type ProposeUnchangedCodingLedgerCommand struct {
	LedgerID, Revision                      int64
	OperationID, OwnerTurnID, OwnerTurnText string
}

// An owner may continue discussing an interrupted task without changing scope.
// Retain the exact approved contents as a proposal; this grants no execution
// authority. A new explicit owner freeze is still required.
func (r *Repository) ProposeUnchangedCodingLedger(ctx context.Context, command ProposeUnchangedCodingLedgerCommand) (CodingLedgerRevision, error) {
	if command.LedgerID < 1 || command.Revision < 1 || strings.TrimSpace(command.OwnerTurnText) == "" {
		return CodingLedgerRevision{}, fmt.Errorf("unchanged proposal requires exact ledger, revision and owner turn")
	}
	if err := validateCodingLedgerOperationID(command.OperationID); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := validateCodingLedgerExternalOwnerTurnID(command.OwnerTurnID); err != nil {
		return CodingLedgerRevision{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	defer tx.Rollback(ctx)
	var revision int64
	var state string
	if err := tx.QueryRow(ctx, `SELECT current_revision,state FROM coding_ledgers WHERE id=$1 FOR UPDATE`, command.LedgerID).Scan(&revision, &state); err != nil {
		return CodingLedgerRevision{}, err
	}
	if state != "revising" || revision != command.Revision {
		return CodingLedgerRevision{}, fmt.Errorf("%w: unchanged proposal requires the current revising ledger", ErrCodingLedgerRevision)
	}
	current, err := readCodingLedgerRevisionTx(ctx, tx, command.LedgerID, revision, CodingLedgerReadOptions{Limit: -1})
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := insertCodingLedgerOwnerTurnTx(ctx, tx, command.LedgerID, command.OwnerTurnID, revision, "planning", command.OwnerTurnText); err != nil {
		return CodingLedgerRevision{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO coding_ledger_revisions(ledger_id,revision,base_revision,operation_id,owner_turn_id,state)
	 VALUES($1,$2,$3,$4,$5,'proposed')`, command.LedgerID, revision+1, revision, command.OperationID, command.OwnerTurnID); err != nil {
		return CodingLedgerRevision{}, err
	}
	current.Ledger.Revision = revision + 1
	if err := persistCodingLedgerSnapshotTx(ctx, tx, command.LedgerID, revision+1, revision, command.OwnerTurnID, current.Ledger, current.Routes); err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := insertCodingLedgerDispositionTx(ctx, tx, command.LedgerID, revision+1, command.OwnerTurnID,
		model.OwnerTurnDisposition{Statements: []model.OwnerStatementDisposition{}}, current.Ledger); err != nil {
		return CodingLedgerRevision{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE coding_ledgers SET state='proposed',updated_at=clock_timestamp() WHERE id=$1`, command.LedgerID); err != nil {
		return CodingLedgerRevision{}, err
	}
	result, err := readCodingLedgerRevisionTx(ctx, tx, command.LedgerID, revision+1, CodingLedgerReadOptions{Limit: -1})
	if err != nil {
		return CodingLedgerRevision{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CodingLedgerRevision{}, err
	}
	result.Applied = true
	return result, nil
}
