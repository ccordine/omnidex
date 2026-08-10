package queue

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

func (r *Repository) ProvisionStepAttemptAuthorizerRole(
	ctx context.Context,
	role string,
) error {
	if ctx == nil || r == nil || r.pool == nil {
		return fmt.Errorf("provision step-attempt authorizer requires PostgreSQL and context")
	}
	if role == "" || role != strings.TrimSpace(role) || len(role) > 63 ||
		!utf8.ValidString(role) || strings.ContainsRune(role, '\x00') ||
		strings.EqualFold(role, "public") {
		return fmt.Errorf("step-attempt authorizer role is not one exact database role")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var currentSchema string
	if err := tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&currentSchema); err != nil {
		return err
	}
	authoritySchema, err := loadInstalledStepAttemptFenceTx(ctx, tx)
	if err != nil {
		return err
	}
	roleOID, err := requireRestrictedStepAttemptRole(
		ctx, tx, role, currentSchema, authoritySchema,
	)
	if err != nil {
		return err
	}
	roleIdentifier := pgx.Identifier{role}.Sanitize()
	schemaIdentifier := pgx.Identifier{authoritySchema}.Sanitize()
	functionIdentifier := pgx.Identifier{authoritySchema, stepAttemptFenceFunction}.Sanitize()
	if _, err := tx.Exec(ctx, `GRANT USAGE ON SCHEMA `+schemaIdentifier+` TO `+roleIdentifier); err != nil {
		return fmt.Errorf("grant runtime schema usage to host role: %w", err)
	}
	if _, err := tx.Exec(ctx, `GRANT EXECUTE ON FUNCTION `+functionIdentifier+
		`(bigint,bigint,bigint,bigint,text) TO `+roleIdentifier); err != nil {
		return fmt.Errorf("grant transactional step-attempt fence to host role: %w", err)
	}
	if _, err := requireRestrictedStepAttemptRole(
		ctx, tx, role, currentSchema, authoritySchema,
	); err != nil {
		return fmt.Errorf("validate provisioned step-attempt authorizer role: %w", err)
	}
	if exactSchema, err := loadInstalledStepAttemptFenceTx(ctx, tx); err != nil ||
		exactSchema != authoritySchema {
		return fmt.Errorf("validate provisioned transactional fence catalog authority")
	}
	var executable bool
	if err := tx.QueryRow(ctx, `
		SELECT has_function_privilege($1::oid,procedures.oid,'EXECUTE')
		FROM pg_proc procedures
		JOIN pg_namespace namespaces ON namespaces.oid=procedures.pronamespace
		WHERE namespaces.nspname=$2 AND procedures.proname=$3
		  AND pg_catalog.oidvectortypes(procedures.proargtypes)=
		      'bigint, bigint, bigint, bigint, text'
	`, roleOID, authoritySchema, stepAttemptFenceFunction).Scan(&executable); err != nil || !executable {
		return fmt.Errorf("host role did not receive the exact transactional fence authority")
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}
