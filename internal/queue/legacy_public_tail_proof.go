package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/db"
	"github.com/jackc/pgx/v5"
)

const legacyCutoverTailProofSavepoint = "legacy_public_current_tail_applicability"

const (
	legacyCutoverTailCleanupTimeout   = 30 * time.Second
	legacyCutoverTailProofTimeout     = 6 * time.Minute
	legacyCutoverTailStatementTimeout = "5min"
)

// proveLegacyCutoverTailApplicability executes every verified migration after
// the frozen preservation release under one savepoint. Rolling the savepoint
// back keeps the frozen ledger and receipt as the only durable cutover authority.
func proveLegacyCutoverTailApplicability(
	ctx context.Context,
	tx pgx.Tx,
	runtimeSchema string,
	source MigrationBundle,
) error {
	if err := source.validate(); err != nil {
		return err
	}
	hasCurrentTail := false
	for _, entry := range source.entries {
		prefix, err := migrationNumericPrefix(entry.name)
		if err != nil {
			return fmt.Errorf(
				"legacy cutover current-tail applicability migration identity %q: %w",
				entry.name, err,
			)
		}
		if prefix > legacyCutoverFinalMigrationPrefix {
			hasCurrentTail = true
			break
		}
	}
	if !hasCurrentTail {
		return fmt.Errorf(
			"legacy cutover current-tail applicability requires verified migrations after frozen prefix 158",
		)
	}
	searchPath, err := db.RuntimeSearchPath(runtimeSchema)
	if err != nil {
		return err
	}
	if err := enforceMigrationTransactionSQLMode(ctx, tx); err != nil {
		return fmt.Errorf("prepare legacy cutover current-tail SQL lexical mode: %w", err)
	}
	sequenceState, err := loadLegacyCutoverSequenceState(ctx, tx, runtimeSchema)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "SAVEPOINT "+legacyCutoverTailProofSavepoint); err != nil {
		return fmt.Errorf("create legacy cutover current-tail applicability savepoint: %w", err)
	}
	proofCtx, cancelProof := context.WithTimeout(
		context.WithoutCancel(ctx), legacyCutoverTailProofTimeout,
	)
	defer cancelProof()
	var proofErr error
	if _, err := tx.Exec(
		proofCtx, `SET LOCAL statement_timeout TO '`+legacyCutoverTailStatementTimeout+`'`,
	); err != nil {
		proofErr = fmt.Errorf("bound legacy cutover current-tail applicability statements: %w", err)
	}
	if proofErr == nil {
		_, err = tx.Exec(proofCtx, `SET LOCAL search_path TO `+searchPath)
	}
	if err != nil {
		proofErr = fmt.Errorf("select legacy cutover current-tail applicability schema: %w", err)
	}
	for _, entry := range source.entries {
		if proofErr != nil {
			break
		}
		prefix, err := migrationNumericPrefix(entry.name)
		if err != nil {
			proofErr = fmt.Errorf(
				"legacy cutover current-tail applicability migration identity %q: %w",
				entry.name, err,
			)
			break
		}
		if prefix <= legacyCutoverFinalMigrationPrefix {
			continue
		}
		body, err := entry.transactionalBody()
		if err != nil {
			proofErr = fmt.Errorf(
				"legacy cutover current-tail applicability failed at migration %s: %w",
				entry.name, err,
			)
			break
		}
		if err := enforceMigrationTransactionSQLMode(proofCtx, tx); err != nil {
			proofErr = fmt.Errorf(
				"legacy cutover current-tail applicability failed to prepare migration %s SQL lexical mode: %w",
				entry.name, err,
			)
			break
		}
		if _, err := tx.Exec(proofCtx, string(body)); err != nil {
			proofErr = fmt.Errorf(
				"legacy cutover current-tail applicability failed at migration %s: %w",
				entry.name, err,
			)
			break
		}
	}

	cleanupCtx, cancelCleanup := context.WithTimeout(
		context.Background(), legacyCutoverTailCleanupTimeout,
	)
	defer cancelCleanup()
	_, rollbackErr := tx.Exec(
		cleanupCtx, "ROLLBACK TO SAVEPOINT "+legacyCutoverTailProofSavepoint,
	)
	_, releaseErr := tx.Exec(
		cleanupCtx, "RELEASE SAVEPOINT "+legacyCutoverTailProofSavepoint,
	)
	if rollbackErr != nil {
		rollbackErr = fmt.Errorf("rollback legacy cutover current-tail applicability proof: %w", rollbackErr)
	}
	if releaseErr != nil {
		releaseErr = fmt.Errorf("release legacy cutover current-tail applicability savepoint: %w", releaseErr)
	}
	sequenceErr := restoreLegacyCutoverSequenceState(
		cleanupCtx, tx, runtimeSchema, sequenceState,
	)
	return errors.Join(proofErr, rollbackErr, releaseErr, sequenceErr)
}
