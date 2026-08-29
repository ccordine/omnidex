package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func verifyLegacyObjectAuthority(ctx context.Context, tx pgx.Tx) error {
	var relationViolations, functionViolations, columnACLs, defaultACLs int
	if err := tx.QueryRow(ctx, `
		SELECT
		 (SELECT COUNT(*) FROM pg_class objects
		  JOIN pg_namespace namespaces ON namespaces.oid=objects.relnamespace
		  WHERE namespaces.nspname='public' AND (
		    objects.relowner<>(SELECT oid FROM pg_roles WHERE rolname=current_user) OR
		    objects.relacl IS NOT NULL
		  ) AND NOT EXISTS (
		    SELECT 1 FROM pg_depend dependency
		    WHERE dependency.classid='pg_class'::regclass AND
		          dependency.objid=objects.oid AND dependency.deptype='e'
		  )),
		 (SELECT COUNT(*) FROM pg_proc procedures
		  JOIN pg_namespace namespaces ON namespaces.oid=procedures.pronamespace
		  WHERE namespaces.nspname='public' AND (
		    procedures.proowner<>(SELECT oid FROM pg_roles WHERE rolname=current_user) OR
		    procedures.proacl IS NOT NULL OR procedures.proconfig IS NOT NULL OR
		    procedures.prosecdef
		  ) AND NOT EXISTS (
		    SELECT 1 FROM pg_depend dependency
		    WHERE dependency.classid='pg_proc'::regclass AND
		          dependency.objid=procedures.oid AND dependency.deptype='e'
		  )),
		 (SELECT COUNT(*) FROM pg_attribute attributes
		  JOIN pg_class objects ON objects.oid=attributes.attrelid
		  JOIN pg_namespace namespaces ON namespaces.oid=objects.relnamespace
		  WHERE namespaces.nspname='public' AND attributes.attacl IS NOT NULL AND
		        NOT EXISTS (
		          SELECT 1 FROM pg_depend dependency
		          WHERE dependency.classid='pg_class'::regclass AND
		                dependency.objid=objects.oid AND dependency.deptype='e'
		        )),
		 (SELECT COUNT(*) FROM pg_default_acl defaults
		  WHERE defaults.defaclnamespace=0 OR defaults.defaclnamespace=(
		          SELECT oid FROM pg_namespace WHERE nspname='public'
		        ))
	`).Scan(&relationViolations, &functionViolations, &columnACLs, &defaultACLs); err != nil {
		return fmt.Errorf("inspect legacy object authority: %w", err)
	}
	if relationViolations != 0 || functionViolations != 0 || columnACLs != 0 || defaultACLs != 0 {
		return fmt.Errorf(
			"legacy public ownership or ACL authority differs: relations=%d functions=%d columns=%d defaults=%d",
			relationViolations, functionViolations, columnACLs, defaultACLs,
		)
	}
	return nil
}

func rejectUnsupportedLegacyObjects(ctx context.Context, tx pgx.Tx) error {
	var unsupportedRelations, standaloneTypes, policies, rules, inheritance int
	var crossSchemaForeignKeys, namespaceObjects int
	if err := tx.QueryRow(ctx, `
		SELECT
		 (SELECT COUNT(*) FROM pg_class objects
		  JOIN pg_namespace namespaces ON namespaces.oid=objects.relnamespace
		  WHERE namespaces.nspname='public' AND objects.relkind NOT IN ('r','S','i') AND
		        NOT EXISTS (
		          SELECT 1 FROM pg_depend dependency
		          WHERE dependency.classid='pg_class'::regclass AND
		                dependency.objid=objects.oid AND dependency.deptype='e'
		        )),
		 (SELECT COUNT(*) FROM pg_type types
		  JOIN pg_namespace namespaces ON namespaces.oid=types.typnamespace
		  WHERE namespaces.nspname='public' AND types.typrelid=0 AND types.typelem=0 AND
		        NOT EXISTS (
		          SELECT 1 FROM pg_depend dependency
		          WHERE dependency.classid='pg_type'::regclass AND
		                dependency.objid=types.oid AND dependency.deptype='e'
		        )),
		 (SELECT COUNT(*) FROM pg_policy policies
		  JOIN pg_class objects ON objects.oid=policies.polrelid
		  JOIN pg_namespace namespaces ON namespaces.oid=objects.relnamespace
		  WHERE namespaces.nspname='public'),
		 (SELECT COUNT(*) FROM pg_rewrite rules
		  JOIN pg_class objects ON objects.oid=rules.ev_class
		  JOIN pg_namespace namespaces ON namespaces.oid=objects.relnamespace
		  WHERE namespaces.nspname='public'),
		 (SELECT COUNT(*) FROM pg_inherits inheritance
		  JOIN pg_class children ON children.oid=inheritance.inhrelid
		  JOIN pg_namespace namespaces ON namespaces.oid=children.relnamespace
		  WHERE namespaces.nspname='public'),
		 (SELECT COUNT(*) FROM pg_constraint constraints
		  JOIN pg_class objects ON objects.oid=constraints.conrelid
		  JOIN pg_namespace namespaces ON namespaces.oid=objects.relnamespace
		  JOIN pg_class referenced ON referenced.oid=constraints.confrelid
		  JOIN pg_namespace referenced_namespaces ON referenced_namespaces.oid=referenced.relnamespace
		  WHERE namespaces.nspname='public' AND constraints.contype='f' AND
		        referenced_namespaces.nspname<>'public'),
		 (SELECT
		    (SELECT COUNT(*) FROM pg_operator o JOIN pg_namespace n ON n.oid=o.oprnamespace
		     WHERE n.nspname='public' AND NOT EXISTS (
		       SELECT 1 FROM pg_depend d WHERE d.classid='pg_operator'::regclass AND
		       d.objid=o.oid AND d.deptype='e')) +
		    (SELECT COUNT(*) FROM pg_opclass o JOIN pg_namespace n ON n.oid=o.opcnamespace
		     WHERE n.nspname='public' AND NOT EXISTS (
		       SELECT 1 FROM pg_depend d WHERE d.classid='pg_opclass'::regclass AND
		       d.objid=o.oid AND d.deptype='e')) +
		    (SELECT COUNT(*) FROM pg_opfamily o JOIN pg_namespace n ON n.oid=o.opfnamespace
		     WHERE n.nspname='public' AND NOT EXISTS (
		       SELECT 1 FROM pg_depend d WHERE d.classid='pg_opfamily'::regclass AND
		       d.objid=o.oid AND d.deptype='e')) +
		    (SELECT COUNT(*) FROM pg_collation o JOIN pg_namespace n ON n.oid=o.collnamespace
		     WHERE n.nspname='public') +
		    (SELECT COUNT(*) FROM pg_conversion o JOIN pg_namespace n ON n.oid=o.connamespace
		     WHERE n.nspname='public'))
	`).Scan(
		&unsupportedRelations, &standaloneTypes, &policies, &rules, &inheritance,
		&crossSchemaForeignKeys, &namespaceObjects,
	); err != nil {
		return fmt.Errorf("inspect unsupported legacy objects: %w", err)
	}
	if unsupportedRelations != 0 || standaloneTypes != 0 || policies != 0 ||
		rules != 0 || inheritance != 0 || crossSchemaForeignKeys != 0 || namespaceObjects != 0 {
		return fmt.Errorf(
			"legacy public contains unsupported objects: relations=%d types=%d policies=%d rules=%d inheritance=%d cross_schema_fks=%d namespace_objects=%d",
			unsupportedRelations, standaloneTypes, policies, rules, inheritance,
			crossSchemaForeignKeys, namespaceObjects,
		)
	}
	return nil
}
