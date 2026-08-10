package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func requireRestrictedStepAttemptRole(
	ctx context.Context,
	tx pgx.Tx,
	role string,
	runtimeSchema string,
	authoritySchema string,
) (uint32, error) {
	var oid uint32
	var superuser, createRole, createDB, replication, bypassRLS bool
	if err := tx.QueryRow(ctx, `
		SELECT oid,rolsuper,rolcreaterole,rolcreatedb,
		       rolreplication,rolbypassrls
		FROM pg_roles WHERE rolname=$1
	`, role).Scan(
		&oid, &superuser, &createRole, &createDB, &replication, &bypassRLS,
	); err != nil {
		return 0, fmt.Errorf("load step-attempt authorizer role: %w", err)
	}
	if superuser || createRole || createDB || replication || bypassRLS {
		return 0, fmt.Errorf("step-attempt authorizer role has forbidden database capabilities")
	}
	var member bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pg_auth_members WHERE member=$1)
	`, oid).Scan(&member); err != nil {
		return 0, err
	}
	if member {
		return 0, fmt.Errorf("step-attempt authorizer role must not inherit another role")
	}
	var hasDataPrivilege bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_class relations
			JOIN pg_namespace namespaces ON namespaces.oid=relations.relnamespace
			WHERE namespaces.nspname=$2 AND (
				(relations.relkind IN ('r','p','v','m','f') AND
				 has_table_privilege($1::oid,relations.oid,
				   'SELECT,INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')) OR
				(relations.relkind='S' AND
				 has_sequence_privilege($1::oid,relations.oid,'USAGE,SELECT,UPDATE'))
			)
		)
	`, oid, runtimeSchema).Scan(&hasDataPrivilege); err != nil {
		return 0, fmt.Errorf("inspect host role queue privileges: %w", err)
	}
	if hasDataPrivilege {
		return 0, fmt.Errorf("step-attempt authorizer role has forbidden queue data privileges")
	}
	var hasUnsafeSchemaPrivilege bool
	if err := tx.QueryRow(ctx, `
		SELECT has_schema_privilege($1::oid,$2,'CREATE') OR
		       has_schema_privilege($1::oid,$3,'CREATE') OR
		       has_schema_privilege($1::oid,$2,'USAGE')
	`, oid, runtimeSchema, authoritySchema).Scan(&hasUnsafeSchemaPrivilege); err != nil {
		return 0, fmt.Errorf("inspect host role schema privileges: %w", err)
	}
	if hasUnsafeSchemaPrivilege {
		return 0, fmt.Errorf("step-attempt authorizer role has forbidden runtime schema privileges")
	}
	var otherAuthorityFunction bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_proc procedures
			JOIN pg_namespace namespaces ON namespaces.oid=procedures.pronamespace
			WHERE namespaces.nspname=$2 AND NOT (
			      procedures.proname=$3 AND
			      pg_catalog.oidvectortypes(procedures.proargtypes)=
			          'bigint, bigint, bigint, bigint, text'
			) AND
			      has_function_privilege($1::oid,procedures.oid,'EXECUTE')
		)
	`, oid, authoritySchema, stepAttemptFenceFunction).Scan(&otherAuthorityFunction); err != nil {
		return 0, fmt.Errorf("inspect host role function privileges: %w", err)
	}
	if otherAuthorityFunction {
		return 0, fmt.Errorf("step-attempt authorizer role has another authority function")
	}
	return oid, nil
}
