package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

const (
	legacyPublicSchema                          = "public"
	legacyMigrationMaximumPrefix                = 24
	legacyExpectedMigrationManifestSHA256       = "178e02e51621c535b648d2b94b60026820f9045dbedd6fca684da23e926bb9f7"
	legacyExpectedCatalogSHA256                 = "83c45ce13170843a21b4780935a6c2c89f5b13fa8c072dff7689db9817ca4dd0"
	legacyExpectedRuntimeCatalogSHA256          = "19848cdd211141f2e4aefb0d0b48bef00b173908c23a6f723269557b610b4c02"
	legacyExpectedExtensionSHA256               = "e5bc2256fa21a1fe4044c755c2e3d1d3cae548048aa07c1ea446497d5581a052"
	legacyExpectedTableCount                    = 42
	legacyExpectedSequenceCount                 = 20
	legacyExpectedIndexCount                    = 129
	legacyExpectedFunctionCount                 = 4
	legacyExpectedTriggerCount                  = 3
	legacyRuntimeBootstrapLockID          int64 = 0x4f4d4e4952545343
)

type legacyExtension struct {
	OID         uint32
	Name        string
	Version     string
	Owner       string
	Relocatable bool
}

func validateLegacyCutoverPreconditions(
	ctx context.Context,
	tx pgx.Tx,
	runtimeSchema string,
	bundle MigrationBundle,
) (legacyCatalogSnapshot, []legacyExtension, error) {
	if err := verifyLegacyDatabaseAuthority(ctx, tx, runtimeSchema); err != nil {
		return legacyCatalogSnapshot{}, nil, err
	}
	if err := rejectConcurrentLegacySessions(ctx, tx); err != nil {
		return legacyCatalogSnapshot{}, nil, err
	}
	if err := lockLegacyPublicTables(ctx, tx); err != nil {
		return legacyCatalogSnapshot{}, nil, err
	}
	if err := verifyLegacyPublicMigrationLedger(ctx, tx, bundle); err != nil {
		return legacyCatalogSnapshot{}, nil, err
	}
	if err := verifyLegacyObjectAuthority(ctx, tx); err != nil {
		return legacyCatalogSnapshot{}, nil, err
	}
	if err := rejectUnsupportedLegacyObjects(ctx, tx); err != nil {
		return legacyCatalogSnapshot{}, nil, err
	}
	extensions, err := loadLegacyExtensions(ctx, tx)
	if err != nil {
		return legacyCatalogSnapshot{}, nil, err
	}
	if legacyExtensionDescriptorSHA256(extensions) != legacyExpectedExtensionSHA256 {
		return legacyCatalogSnapshot{}, nil, fmt.Errorf(
			"legacy public extension descriptors differ from the frozen equivalence attestation",
		)
	}
	snapshot, err := loadLegacyCatalogSnapshot(ctx, tx, legacyPublicSchema)
	if err != nil {
		return legacyCatalogSnapshot{}, nil, err
	}
	if snapshot.SHA256 != legacyExpectedCatalogSHA256 ||
		snapshot.Tables != legacyExpectedTableCount ||
		snapshot.Sequences != legacyExpectedSequenceCount ||
		snapshot.Indexes != legacyExpectedIndexCount ||
		snapshot.Functions != legacyExpectedFunctionCount ||
		snapshot.Triggers != legacyExpectedTriggerCount {
		return legacyCatalogSnapshot{}, nil, fmt.Errorf(
			"legacy public catalog differs from the frozen 001..024 equivalence attestation: sha256=%s shape=%d/%d/%d/%d/%d",
			snapshot.SHA256, snapshot.Tables, snapshot.Sequences, snapshot.Indexes,
			snapshot.Functions, snapshot.Triggers,
		)
	}
	return snapshot, extensions, nil
}

func verifyLegacyDatabaseAuthority(ctx context.Context, tx pgx.Tx, runtimeSchema string) error {
	var currentUser, sessionUser, databaseOwner, publicOwner string
	var runtimeExists bool
	if err := tx.QueryRow(ctx, `
		SELECT current_user,session_user,pg_get_userbyid(databases.datdba),
		       pg_get_userbyid(public_namespace.nspowner),
		       EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)
		FROM pg_database databases
		JOIN pg_namespace public_namespace ON public_namespace.nspname='public'
		WHERE databases.datname=current_database()
	`, runtimeSchema).Scan(
		&currentUser, &sessionUser, &databaseOwner, &publicOwner, &runtimeExists,
	); err != nil {
		return fmt.Errorf("inspect legacy database authority: %w", err)
	}
	if currentUser == "" || currentUser != sessionUser || currentUser != databaseOwner {
		return fmt.Errorf("legacy cutover requires the exact database owner session")
	}
	if publicOwner != "pg_database_owner" {
		return fmt.Errorf("legacy public schema owner differs from pg_database_owner")
	}
	if runtimeExists {
		return fmt.Errorf("runtime schema %q already exists beside legacy public state", runtimeSchema)
	}
	var exactACL bool
	if err := tx.QueryRow(ctx, legacyPublicACLCheckSQL).Scan(&exactACL); err != nil {
		return fmt.Errorf("inspect legacy public schema ACL: %w", err)
	}
	if !exactACL {
		return fmt.Errorf("legacy public schema ACL differs from the standard authority")
	}
	return nil
}

const legacyPublicACLCheckSQL = `
WITH namespace AS (
 SELECT oid,nspowner,nspacl FROM pg_namespace WHERE nspname='public'
), entries AS (
 SELECT acl.grantee,acl.grantor,acl.privilege_type,acl.is_grantable,namespace.nspowner
 FROM namespace,LATERAL aclexplode(COALESCE(
   namespace.nspacl,acldefault('n',namespace.nspowner)
 )) acl
)
SELECT COUNT(*)=3 AND
       COUNT(*) FILTER (WHERE grantee=0 AND grantor=nspowner AND
         privilege_type='USAGE' AND NOT is_grantable)=1 AND
       COUNT(*) FILTER (WHERE grantee=nspowner AND grantor=nspowner AND
         privilege_type='USAGE' AND NOT is_grantable)=1 AND
       COUNT(*) FILTER (WHERE grantee=nspowner AND grantor=nspowner AND
         privilege_type='CREATE' AND NOT is_grantable)=1
FROM entries`

func rejectConcurrentLegacySessions(ctx context.Context, tx pgx.Tx) error {
	var count int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_stat_activity
		WHERE datid=(SELECT oid FROM pg_database WHERE datname=current_database()) AND
		      pid<>pg_backend_pid() AND backend_type='client backend'
	`).Scan(&count); err != nil {
		return fmt.Errorf("inspect legacy database sessions: %w", err)
	}
	if count != 0 {
		return fmt.Errorf("legacy cutover requires zero other database sessions; found %d", count)
	}
	return nil
}

func lockLegacyPublicTables(ctx context.Context, tx pgx.Tx) error {
	rows, err := tx.Query(ctx, `
		SELECT objects.relname FROM pg_class objects
		JOIN pg_namespace namespaces ON namespaces.oid=objects.relnamespace
		WHERE namespaces.nspname='public' AND objects.relkind IN ('r','p') AND
		      NOT EXISTS (
		        SELECT 1 FROM pg_depend dependency
		        WHERE dependency.classid='pg_class'::regclass AND
		              dependency.objid=objects.oid AND dependency.deptype='e'
		      )
		ORDER BY objects.relname
	`)
	if err != nil {
		return fmt.Errorf("enumerate legacy public tables: %w", err)
	}
	names := make([]string, 0, legacyExpectedTableCount)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return fmt.Errorf("scan legacy public table: %w", err)
		}
		names = append(names, pgx.Identifier{legacyPublicSchema, name}.Sanitize())
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("iterate legacy public tables: %w", err)
	}
	if len(names) == 0 {
		return fmt.Errorf("legacy public schema contains no lockable application tables")
	}
	if _, err := tx.Exec(ctx, "LOCK TABLE "+strings.Join(names, ",")+" IN ACCESS EXCLUSIVE MODE"); err != nil {
		return fmt.Errorf("lock legacy public tables: %w", err)
	}
	return nil
}
