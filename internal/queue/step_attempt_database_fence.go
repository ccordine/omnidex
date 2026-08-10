package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const stepAttemptFenceFunction = "omnidex_authorize_step_attempt_transaction_v1"

type stepAttemptFenceCandidate struct {
	schema   string
	nonOwned bool
}

func resolveStepAttemptFenceTransaction(ctx context.Context, tx pgx.Tx) (string, error) {
	rows, err := tx.Query(ctx, `
		SELECT namespaces.nspname,
		       procedures.proowner=(SELECT oid FROM pg_roles WHERE rolname=current_user)
		FROM pg_proc procedures
		JOIN pg_namespace namespaces ON namespaces.oid=procedures.pronamespace
		WHERE procedures.proname=$1
		  AND pg_catalog.oidvectortypes(procedures.proargtypes)=
		      'bigint, bigint, bigint, bigint, text'
		  AND procedures.prorettype='boolean'::regtype
		  AND procedures.prosecdef
		  AND namespaces.nspname !~ '^pg_' AND namespaces.nspname<>'information_schema'
		  AND has_function_privilege(current_user,procedures.oid,'EXECUTE')
		ORDER BY namespaces.nspname
	`, stepAttemptFenceFunction)
	if err != nil {
		return "", fmt.Errorf("resolve transactional step-attempt fence: %w", err)
	}
	defer rows.Close()

	nonOwned := make([]stepAttemptFenceCandidate, 0, 1)
	for rows.Next() {
		var candidate stepAttemptFenceCandidate
		var owned bool
		if err := rows.Scan(&candidate.schema, &owned); err != nil {
			return "", err
		}
		candidate.nonOwned = !owned
		if candidate.nonOwned {
			nonOwned = append(nonOwned, candidate)
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(nonOwned) == 1 {
		return nonOwned[0].schema, nil
	}
	if len(nonOwned) == 0 {
		return loadInstalledStepAttemptFenceTx(ctx, tx)
	}
	return "", fmt.Errorf(
		"transactional step-attempt fence authority is ambiguous or unavailable",
	)
}

func loadInstalledStepAttemptFenceTx(ctx context.Context, tx pgx.Tx) (string, error) {
	var runtimeSchema string
	if err := tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&runtimeSchema); err != nil {
		return "", err
	}
	registry := pgx.Identifier{runtimeSchema, "step_attempt_transaction_fence_authority"}.Sanitize()
	var schema, function, arguments string
	if err := tx.QueryRow(ctx, `
		SELECT authority_schema,function_name,function_arguments FROM `+registry+`
		WHERE singleton
	`).Scan(&schema, &function, &arguments); err != nil {
		return "", fmt.Errorf("load installed transactional fence authority: %w", err)
	}
	if function != stepAttemptFenceFunction ||
		arguments != "bigint, bigint, bigint, bigint, text" {
		return "", fmt.Errorf("installed transactional fence identity changed")
	}
	var exact bool
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)=1
		FROM pg_proc procedures
		JOIN pg_namespace namespaces ON namespaces.oid=procedures.pronamespace
		WHERE namespaces.nspname=$1 AND procedures.proname=$2
		  AND pg_catalog.oidvectortypes(procedures.proargtypes)=$3
		  AND procedures.prorettype='boolean'::regtype
		  AND procedures.prosecdef
		  AND procedures.proowner=(SELECT oid FROM pg_roles WHERE rolname=current_user)
		  AND procedures.proconfig @>
		      ARRAY['search_path=pg_catalog, ' || pg_catalog.quote_ident($4)]
	`, schema, function, arguments, runtimeSchema).Scan(&exact); err != nil || !exact {
		return "", fmt.Errorf("installed transactional fence catalog authority changed")
	}
	return schema, nil
}

func callStepAttemptFenceTransaction(
	ctx context.Context,
	tx pgx.Tx,
	schema string,
	authority model.StepAttemptAuthority,
) error {
	qualified := pgx.Identifier{schema, stepAttemptFenceFunction}.Sanitize()
	var authorized bool
	if err := tx.QueryRow(ctx, `SELECT `+qualified+`($1,$2,$3,$4,$5)`,
		authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID,
	).Scan(&authorized); err != nil {
		return fmt.Errorf("execute transactional step-attempt fence: %w", err)
	}
	if !authorized {
		return staleStepAttemptError(authority, "database transaction fence rejected authority", nil)
	}
	return nil
}
