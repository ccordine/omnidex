package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) CodingLedgerPlanningAudit(ctx context.Context, ledgerID int64, auditID string) (CodingLedgerPlanningAudit, bool, error) {
	if r == nil || r.pool == nil || ledgerID <= 0 || strings.TrimSpace(auditID) == "" {
		return CodingLedgerPlanningAudit{}, false, fmt.Errorf("planning audit lookup requires exact ledger and identity")
	}
	var result CodingLedgerPlanningAudit
	err := r.pool.QueryRow(ctx, `
		SELECT ledger_id,audit_id,revision,ready,explanation
		FROM coding_ledger_planning_audits
		WHERE ledger_id=$1 AND audit_id=$2
	`, ledgerID, auditID).Scan(&result.LedgerID, &result.AuditID, &result.Revision, &result.Ready, &result.Explanation)
	if errors.Is(err, pgx.ErrNoRows) {
		return CodingLedgerPlanningAudit{}, false, nil
	}
	if err != nil {
		return CodingLedgerPlanningAudit{}, false, fmt.Errorf("read planning audit: %w", err)
	}
	return result, true, nil
}

func (r *Repository) RecordCodingLedgerPlanningAudit(ctx context.Context, command RecordCodingLedgerPlanningAuditCommand) error {
	if r == nil || r.pool == nil || command.LedgerID <= 0 || command.Revision <= 0 ||
		strings.TrimSpace(command.AuditID) == "" || strings.TrimSpace(command.Explanation) == "" {
		return fmt.Errorf("planning audit requires exact ledger, revision, identity, and explanation")
	}
	if err := validateCodingLedgerOperationID(command.AuditID); err != nil {
		return err
	}
	result, err := r.pool.Exec(ctx, `
		INSERT INTO coding_ledger_planning_audits(ledger_id,audit_id,revision,ready,explanation)
		SELECT $1,$2,$3,$4,$5
		FROM coding_ledgers WHERE id=$1 AND current_revision=$3 AND state='proposed'
		ON CONFLICT (ledger_id,audit_id) DO NOTHING
	`, command.LedgerID, command.AuditID, command.Revision, command.Ready, command.Explanation)
	if err != nil {
		return fmt.Errorf("record planning audit: %w", err)
	}
	if result.RowsAffected() != 1 {
		var revision int64
		var ready bool
		var explanation string
		if err := r.pool.QueryRow(ctx, `SELECT revision,ready,explanation FROM coding_ledger_planning_audits WHERE ledger_id=$1 AND audit_id=$2`, command.LedgerID, command.AuditID).Scan(&revision, &ready, &explanation); err != nil {
			return fmt.Errorf("planning audit identity is stale or already used: %w", err)
		}
		if revision != command.Revision || ready != command.Ready || explanation != command.Explanation {
			return fmt.Errorf("planning audit identity was already used by a different judgment")
		}
	}
	return nil
}
