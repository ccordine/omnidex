package queue

import (
	"context"
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5"
)

func verifyRenamedLegacyInventory(
	ctx context.Context,
	tx pgx.Tx,
	runtimeSchema string,
	legacy legacyCatalogSnapshot,
	extensions []legacyExtension,
) error {
	renamed, err := loadLegacyCatalogSnapshot(ctx, tx, runtimeSchema)
	if err != nil {
		return err
	}
	if renamed.SHA256 != legacy.SHA256 ||
		renamed.ObjectOIDsSHA256 != legacy.ObjectOIDsSHA256 {
		return fmt.Errorf(
			"schema rename did not preserve the exact application catalog and OIDs: catalog=%s/%s oids=%s/%s",
			legacy.SHA256, renamed.SHA256, legacy.ObjectOIDsSHA256, renamed.ObjectOIDsSHA256,
		)
	}
	currentExtensions, err := loadLegacyExtensions(ctx, tx)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(currentExtensions, extensions) {
		return fmt.Errorf("extension relocation did not preserve exact extension identity")
	}
	return verifyCutoverSchemaAuthority(ctx, tx, runtimeSchema)
}

func verifyCutoverSchemaAuthority(ctx context.Context, tx pgx.Tx, runtimeSchema string) error {
	var runtimeOwned, runtimePublicAccess, publicAppObjects, exactPublicACL bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(
		 SELECT 1 FROM pg_namespace WHERE nspname=$1 AND
		 nspowner=(SELECT oid FROM pg_roles WHERE rolname=current_user)
		),EXISTS(
		 SELECT 1 FROM aclexplode(COALESCE(
		   (SELECT nspacl FROM pg_namespace WHERE nspname=$1),
		   acldefault('n',(SELECT nspowner FROM pg_namespace WHERE nspname=$1))
		 )) acl WHERE acl.grantee=0
		),EXISTS(
		 SELECT 1 FROM pg_class objects JOIN pg_namespace namespaces
		 ON namespaces.oid=objects.relnamespace
		 WHERE namespaces.nspname='public' AND NOT EXISTS (
		   SELECT 1 FROM pg_depend dependency
		   WHERE dependency.classid='pg_class'::regclass AND
		         dependency.objid=objects.oid AND dependency.deptype='e'
		 )
		),(`+legacyPublicACLCheckSQL+`)
	`, runtimeSchema).Scan(
		&runtimeOwned, &runtimePublicAccess, &publicAppObjects, &exactPublicACL,
	); err != nil {
		return fmt.Errorf("verify cutover schema authority: %w", err)
	}
	if !runtimeOwned || runtimePublicAccess || publicAppObjects || !exactPublicACL {
		return fmt.Errorf("cutover schema owner, ACL, or public inventory postcondition failed")
	}
	return nil
}

func verifyCompletedLegacyCutover(
	ctx context.Context,
	tx pgx.Tx,
	runtimeSchema string,
	bundle MigrationBundle,
	legacyExists, runtimeExists bool,
) (LegacyPublicCutoverReceipt, error) {
	if legacyExists || !runtimeExists {
		return LegacyPublicCutoverReceipt{}, fmt.Errorf(
			"database is neither the exact legacy public state nor a completed cutover",
		)
	}
	if err := rejectConcurrentLegacySessions(ctx, tx); err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	if err := lockSchemaApplicationTables(ctx, tx, runtimeSchema); err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	var raw string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(obj_description(oid,'pg_namespace'),'')
		FROM pg_namespace WHERE nspname=$1`, runtimeSchema).Scan(&raw); err != nil {
		return LegacyPublicCutoverReceipt{}, fmt.Errorf("load durable legacy cutover receipt: %w", err)
	}
	receipt, err := decodeLegacyPublicCutoverReceipt(raw)
	if err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	if err := receipt.validate(bundle, runtimeSchema); err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	if err := verifyFinalLegacyCutoverState(ctx, tx, runtimeSchema, bundle, receipt); err != nil {
		return LegacyPublicCutoverReceipt{}, err
	}
	return receipt, nil
}

func verifyFinalLegacyCutoverState(
	ctx context.Context,
	tx pgx.Tx,
	runtimeSchema string,
	bundle MigrationBundle,
	receipt LegacyPublicCutoverReceipt,
) error {
	if err := verifyCutoverSchemaAuthority(ctx, tx, runtimeSchema); err != nil {
		return err
	}
	searchPath := pgx.Identifier{runtimeSchema}.Sanitize() + ",public,pg_catalog"
	if _, err := tx.Exec(ctx, `SET LOCAL search_path TO `+searchPath); err != nil {
		return fmt.Errorf("select completed runtime schema: %w", err)
	}
	if _, err := loadInstalledStepAttemptFenceTx(ctx, tx); err != nil {
		return fmt.Errorf("verify completed cutover transaction fence: %w", err)
	}
	var runtimeOID uint32
	if err := tx.QueryRow(ctx, `SELECT oid FROM pg_namespace WHERE nspname=$1`, runtimeSchema).Scan(
		&runtimeOID,
	); err != nil || runtimeOID != receipt.RuntimeSchemaOID {
		return fmt.Errorf("completed cutover runtime schema OID differs from its receipt")
	}
	applied, err := loadAppliedFileMigrationsTx(ctx, tx)
	if err != nil {
		return err
	}
	if err := validateAppliedFileMigrations(bundle, applied, true); err != nil {
		return fmt.Errorf("completed cutover migration authority differs: %w", err)
	}
	catalog, err := loadLegacyCatalogSnapshot(ctx, tx, runtimeSchema)
	if err != nil {
		return err
	}
	if catalog.SHA256 != receipt.RuntimeCatalogSHA256 ||
		catalog.ObjectOIDsSHA256 != receipt.ObjectOIDsSHA256 {
		return fmt.Errorf("completed cutover runtime catalog or object OIDs differ from its receipt")
	}
	extensions, err := loadLegacyExtensions(ctx, tx)
	if err != nil {
		return err
	}
	if legacyExtensionSHA256(extensions) != receipt.ExtensionsSHA256 {
		return fmt.Errorf("completed cutover extensions differ from their receipt")
	}
	return nil
}

func lockSchemaApplicationTables(ctx context.Context, tx pgx.Tx, schema string) error {
	rows, err := tx.Query(ctx, `
		SELECT objects.relname FROM pg_class objects
		JOIN pg_namespace namespaces ON namespaces.oid=objects.relnamespace
		WHERE namespaces.nspname=$1 AND objects.relkind IN ('r','p')
		ORDER BY objects.relname
	`, schema)
	if err != nil {
		return fmt.Errorf("enumerate completed runtime tables: %w", err)
	}
	identifiers := make([]string, 0, 128)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		identifiers = append(identifiers, pgx.Identifier{schema, name}.Sanitize())
	}
	rows.Close()
	if len(identifiers) == 0 {
		return fmt.Errorf("completed runtime schema contains no application tables")
	}
	command := "LOCK TABLE "
	for index, identifier := range identifiers {
		if index > 0 {
			command += ","
		}
		command += identifier
	}
	command += " IN ACCESS EXCLUSIVE MODE"
	if _, err := tx.Exec(ctx, command); err != nil {
		return fmt.Errorf("lock completed runtime tables: %w", err)
	}
	return nil
}
