package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func bindLegacyMigrationAuthority(
	ctx context.Context,
	tx pgx.Tx,
	bundle MigrationBundle,
) error {
	legacy, err := legacyMigrationEntries(bundle)
	if err != nil {
		return err
	}
	runtimeSchema, err := exactCutoverRuntimeSchema(ctx, tx)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		ALTER TABLE `+pgx.Identifier{runtimeSchema, "schema_migrations"}.Sanitize()+`
		 ADD COLUMN sha256 TEXT;
		ALTER TABLE `+pgx.Identifier{runtimeSchema, "schema_migrations"}.Sanitize()+`
		 ADD COLUMN manifest_sha256 TEXT;
	`); err != nil {
		return fmt.Errorf("add attested migration authority columns: %w", err)
	}
	for _, entry := range legacy {
		result, err := tx.Exec(ctx, `UPDATE `+
			pgx.Identifier{runtimeSchema, "schema_migrations"}.Sanitize()+
			` SET sha256=$2,manifest_sha256=$3 WHERE filename=$1`,
			entry.name, entry.sha256, bundle.manifestSHA256)
		if err != nil {
			return fmt.Errorf("bind attested legacy migration %s: %w", entry.name, err)
		}
		if result.RowsAffected() != 1 {
			return fmt.Errorf("attested legacy migration %s did not bind exactly one row", entry.name)
		}
	}
	if _, err := tx.Exec(ctx, `
		ALTER TABLE `+pgx.Identifier{runtimeSchema, "schema_migrations"}.Sanitize()+`
		 ALTER COLUMN sha256 SET NOT NULL;
		ALTER TABLE `+pgx.Identifier{runtimeSchema, "schema_migrations"}.Sanitize()+`
		 ALTER COLUMN manifest_sha256 SET NOT NULL;
	`); err != nil {
		return fmt.Errorf("seal attested migration authority columns: %w", err)
	}
	return nil
}

func applyPostLegacyMigrations(
	ctx context.Context,
	tx pgx.Tx,
	bundle MigrationBundle,
) error {
	runtimeSchema, err := exactCutoverRuntimeSchema(ctx, tx)
	if err != nil {
		return err
	}
	ledger := pgx.Identifier{runtimeSchema, "schema_migrations"}.Sanitize()
	qualifiedSearchPath := `SET search_path TO ` +
		pgx.Identifier{runtimeSchema}.Sanitize() + `,public,pg_catalog;`
	for _, entry := range bundle.entries {
		prefix, err := migrationNumericPrefix(entry.name)
		if err != nil {
			return err
		}
		if prefix <= legacyMigrationMaximumPrefix {
			continue
		}
		body, err := entry.transactionalBody()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, qualifiedSearchPath+"\n"+string(body)); err != nil {
			return fmt.Errorf("apply preserved-state migration %s: %w", entry.name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO `+ledger+`
			(filename,sha256,manifest_sha256) VALUES ($1,$2,$3)`,
			entry.name, entry.sha256, bundle.manifestSHA256); err != nil {
			return fmt.Errorf("record preserved-state migration %s: %w", entry.name, err)
		}
	}
	applied, err := scanAppliedLegacyFileMigrations(ctx, tx, ledger)
	if err != nil {
		return err
	}
	if err := validateAppliedFileMigrations(bundle, applied, true); err != nil {
		return fmt.Errorf("validate preserved-state migration ledger: %w", err)
	}
	return nil
}

func scanAppliedLegacyFileMigrations(
	ctx context.Context,
	tx pgx.Tx,
	ledger string,
) (map[string]appliedMigration, error) {
	rows, err := tx.Query(ctx, `SELECT filename,sha256,manifest_sha256 FROM `+
		ledger+` ORDER BY filename`)
	if err != nil {
		return nil, fmt.Errorf("load preserved-state migration ledger: %w", err)
	}
	defer rows.Close()
	applied := map[string]appliedMigration{}
	for rows.Next() {
		var name string
		var value appliedMigration
		if err := rows.Scan(&name, &value.sha256, &value.manifestSHA256); err != nil {
			return nil, err
		}
		applied[name] = value
	}
	return applied, rows.Err()
}

func exactCutoverRuntimeSchema(ctx context.Context, tx pgx.Tx) (string, error) {
	var schema string
	if err := tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		return "", fmt.Errorf("load cutover runtime schema: %w", err)
	}
	if schema == "" || schema == legacyPublicSchema {
		return "", fmt.Errorf("post-legacy migrations require the dedicated runtime schema")
	}
	return schema, nil
}

func renameLegacySchemaAndRestorePublic(
	ctx context.Context,
	tx pgx.Tx,
	runtimeSchema string,
	extensions []legacyExtension,
) (uint32, error) {
	var legacySchemaOID uint32
	if err := tx.QueryRow(ctx, `SELECT oid FROM pg_namespace WHERE nspname='public'`).Scan(
		&legacySchemaOID,
	); err != nil {
		return 0, fmt.Errorf("load legacy public schema OID: %w", err)
	}
	if _, err := tx.Exec(ctx, `ALTER SCHEMA public RENAME TO `+
		pgx.Identifier{runtimeSchema}.Sanitize()); err != nil {
		return 0, fmt.Errorf("rename legacy public schema: %w", err)
	}
	if _, err := tx.Exec(ctx, `ALTER SCHEMA `+pgx.Identifier{runtimeSchema}.Sanitize()+
		` OWNER TO CURRENT_USER`); err != nil {
		return 0, fmt.Errorf("bind runtime schema owner: %w", err)
	}
	if _, err := tx.Exec(ctx, `REVOKE ALL ON SCHEMA `+
		pgx.Identifier{runtimeSchema}.Sanitize()+` FROM PUBLIC`); err != nil {
		return 0, fmt.Errorf("revoke runtime schema public access: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		CREATE SCHEMA public AUTHORIZATION pg_database_owner;
		GRANT USAGE ON SCHEMA public TO PUBLIC;
		COMMENT ON SCHEMA public IS 'standard public schema';
	`); err != nil {
		return 0, fmt.Errorf("recreate standard public schema: %w", err)
	}
	for _, extension := range extensions {
		if _, err := tx.Exec(ctx, `ALTER EXTENSION `+
			pgx.Identifier{extension.Name}.Sanitize()+` SET SCHEMA public`); err != nil {
			return 0, fmt.Errorf("restore extension %s to public: %w", extension.Name, err)
		}
	}
	searchPath := pgx.Identifier{runtimeSchema}.Sanitize() + ",public,pg_catalog"
	if _, err := tx.Exec(ctx, `SET LOCAL search_path TO `+searchPath); err != nil {
		return 0, fmt.Errorf("select renamed runtime schema: %w", err)
	}
	var runtimeOID uint32
	if err := tx.QueryRow(ctx, `SELECT oid FROM pg_namespace WHERE nspname=$1`, runtimeSchema).Scan(
		&runtimeOID,
	); err != nil || runtimeOID != legacySchemaOID {
		return 0, fmt.Errorf("renamed runtime schema did not preserve its exact OID")
	}
	return runtimeOID, nil
}

func installLegacyCutoverReceipt(
	ctx context.Context,
	tx pgx.Tx,
	runtimeSchema string,
	receipt LegacyPublicCutoverReceipt,
) error {
	raw, err := receipt.exactJSON()
	if err != nil {
		return err
	}
	literal := "'" + strings.ReplaceAll(raw, "'", "''") + "'"
	if _, err := tx.Exec(ctx, `COMMENT ON SCHEMA `+
		pgx.Identifier{runtimeSchema}.Sanitize()+` IS `+literal); err != nil {
		return fmt.Errorf("install legacy public cutover receipt: %w", err)
	}
	return nil
}
