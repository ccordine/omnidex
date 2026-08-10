package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func validateOfflineTableAuthority(
	ctx context.Context,
	tx pgx.Tx,
	schema string,
	role string,
	requireSequences bool,
) error {
	var usage, create bool
	if err := tx.QueryRow(ctx,
		`SELECT has_schema_privilege($1,$2,'USAGE'), has_schema_privilege($1,$2,'CREATE')`,
		role, schema,
	).Scan(&usage, &create); err != nil {
		return err
	}
	if !usage || create {
		return fmt.Errorf("process role schema authority is not exact")
	}
	var tableCount int
	var tablesExact bool
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(BOOL_AND(
			has_table_privilege($1,c.oid,'SELECT') AND
			has_table_privilege($1,c.oid,'INSERT') AND
			has_table_privilege($1,c.oid,'UPDATE') AND
			has_table_privilege($1,c.oid,'DELETE') AND
			NOT has_table_privilege($1,c.oid,'TRUNCATE') AND
			NOT has_table_privilege($1,c.oid,'REFERENCES') AND
			NOT has_table_privilege($1,c.oid,'TRIGGER')
		),FALSE)
		FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=$2 AND c.relkind IN ('r','p','v','m','f')`,
		role, schema,
	).Scan(&tableCount, &tablesExact); err != nil {
		return err
	}
	if tableCount == 0 || !tablesExact {
		return fmt.Errorf("process role table authority is not exact")
	}
	var sequenceCount int
	var sequencesGranted, sequencesAbsent bool
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(BOOL_AND(
			has_sequence_privilege($1,c.oid,'USAGE') AND
			has_sequence_privilege($1,c.oid,'SELECT') AND
			has_sequence_privilege($1,c.oid,'UPDATE')
		),TRUE), COALESCE(BOOL_AND(
			NOT has_sequence_privilege($1,c.oid,'USAGE') AND
			NOT has_sequence_privilege($1,c.oid,'SELECT') AND
			NOT has_sequence_privilege($1,c.oid,'UPDATE')
		),TRUE)
		FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname=$2 AND c.relkind='S'`,
		role, schema,
	).Scan(&sequenceCount, &sequencesGranted, &sequencesAbsent); err != nil {
		return err
	}
	if requireSequences {
		if sequenceCount == 0 || !sequencesGranted {
			return fmt.Errorf("runtime process role sequence authority is not exact")
		}
		return nil
	}
	if !sequencesAbsent {
		return fmt.Errorf("host process role received sequence authority")
	}
	return nil
}
