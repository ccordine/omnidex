package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const (
	workspaceMutationProjectLocationInsertFunctionSHA256    = "e76c9d7ad78aad3ca57375b9c914f100be8ce16395db7624111f6982a39f3352"
	workspaceMutationProjectLocationCurrentFunctionSHA256   = "f53f54994b7c4bd61d40831af93607b6f2053bc923af58672f548d22bd997941"
	workspaceMutationProjectLocationUpdateFunctionSHA256    = "3f2b6f2d24f2ce88260f46d7cc2c27377d4c8d2150d78dc22751569608b92d5b"
	workspaceMutationProjectLocationImmutableFunctionSHA256 = "fdcea1012ffbb9b150e31f9f01ef49819e07ec2996c7d330de002b993d9c7e8d"
	workspaceMutationProjectLocationChangeGuardSHA256       = "07ab38f846278a6f1385127428e02244ec7e898e2e6a2609de719ef89b793a25"
	workspaceMutationProjectLocationConstraintSHA256        = "5d9402d516d8d5ec5e2f71780580298aad7affcfccdf9794797713305e827872"
)

// validateWorkspaceMutationProjectLocationAuthority prevents production from
// starting with a weakened host-location/runtime-root persistence boundary.
func (r *Repository) validateWorkspaceMutationProjectLocationAuthority(
	ctx context.Context,
) error {
	if r == nil || r.pool == nil || ctx == nil {
		return fmt.Errorf("workspace mutation project-location validation requires PostgreSQL and context")
	}
	var databaseAuthority bool
	err := r.pool.QueryRow(ctx, `
		WITH expected_functions(signature,digest,arguments,return_type,language_name) AS (
			VALUES
			('validate_workspace_mutation_insert()', $1, 0, 'trigger', 'plpgsql'),
			('workspace_mutation_current_authority_valid(workspace_mutation_operations)', $2, 1, 'boolean', 'sql'),
			('validate_workspace_mutation_update()', $3, 0, 'trigger', 'plpgsql'),
			('prevent_workspace_mutation_project_location_change()', $4, 0, 'trigger', 'plpgsql'),
			('prevent_project_location_change_during_active_work()', $5, 0, 'trigger', 'plpgsql')
		), function_authority AS (
			SELECT COUNT(*)=5 AND COALESCE(bool_and(
				procedure.oid IS NOT NULL AND
				encode(digest(convert_to(procedure.prosrc,'UTF8'),'sha256'),'hex')=
					expected_functions.digest AND
				procedure.prokind='f' AND procedure.pronargs=expected_functions.arguments AND
				procedure.pronargdefaults=0 AND
				procedure.prorettype::regtype::text=expected_functions.return_type AND
				NOT procedure.proretset AND language.lanname=expected_functions.language_name AND
				procedure.provolatile='v' AND procedure.proparallel='u' AND
				NOT procedure.proisstrict AND NOT procedure.prosecdef AND
				NOT procedure.proleakproof AND procedure.proconfig IS NULL
			),FALSE) AS exact
			FROM expected_functions
			LEFT JOIN pg_proc AS procedure ON procedure.oid=to_regprocedure(
				current_schema() || '.' || expected_functions.signature
			)
			LEFT JOIN pg_language AS language ON language.oid=procedure.prolang
		), expected_triggers(relation_name,name,function_signature,type_bits) AS (
			VALUES
			('workspace_mutation_operations','workspace_mutation_insert_validate',
			 'validate_workspace_mutation_insert()',7),
			('workspace_mutation_operations','workspace_mutation_update_validate',
			 'validate_workspace_mutation_update()',19),
			('workspace_mutation_operations','workspace_mutation_project_location_immutable',
			 'prevent_workspace_mutation_project_location_change()',19),
			('projects','projects_active_work_location_guard',
			 'prevent_project_location_change_during_active_work()',19)
		), trigger_authority AS (
			SELECT COUNT(*)=4 AND COALESCE(bool_and(
				trigger_row.oid IS NOT NULL AND trigger_row.tgenabled='O' AND
				trigger_row.tgtype=expected_triggers.type_bits AND
				trigger_row.tgattr::text='' AND trigger_row.tgqual IS NULL AND
				trigger_row.tgconstraint=0 AND trigger_row.tgconstrrelid=0 AND
				trigger_row.tgconstrindid=0 AND NOT trigger_row.tgdeferrable AND
				NOT trigger_row.tginitdeferred AND trigger_row.tgnargs=0 AND
				octet_length(trigger_row.tgargs)=0 AND trigger_row.tgoldtable IS NULL AND
				trigger_row.tgnewtable IS NULL AND NOT trigger_row.tgisinternal AND
				trigger_row.tgfoid=to_regprocedure(
					current_schema() || '.' || expected_triggers.function_signature
				)
			),FALSE) AS exact
			FROM expected_triggers
			LEFT JOIN pg_trigger AS trigger_row ON
				trigger_row.tgrelid=to_regclass(
					current_schema() || '.' || expected_triggers.relation_name
				) AND
				trigger_row.tgname=expected_triggers.name
		), column_authority AS (
			SELECT COUNT(*)=1 AND COALESCE(bool_and(
				attribute.attnum>0 AND NOT attribute.attisdropped AND
				attribute.atttypid='text'::regtype AND attribute.attnotnull AND
				NOT attribute.atthasdef AND attribute.attidentity='' AND
				attribute.attgenerated=''
			),FALSE) AS exact
			FROM pg_attribute AS attribute
			WHERE attribute.attrelid='workspace_mutation_operations'::regclass
			  AND attribute.attname='project_location'
		), constraint_authority AS (
			SELECT COUNT(*)=1 AND COALESCE(bool_and(
				constraint_row.contype='c' AND constraint_row.convalidated AND
				constraint_row.conislocal AND constraint_row.coninhcount=0 AND
				NOT constraint_row.condeferrable AND NOT constraint_row.condeferred AND
				NOT constraint_row.connoinherit AND array_length(constraint_row.conkey,1)=1 AND
				(
					SELECT attribute.attname
					FROM pg_attribute AS attribute
					WHERE attribute.attrelid=constraint_row.conrelid AND
					      attribute.attnum=constraint_row.conkey[1]
				)='project_location' AND
				encode(digest(convert_to(
					pg_get_constraintdef(constraint_row.oid,true),'UTF8'
				),'sha256'),'hex')=$6
			),FALSE) AS exact
			FROM pg_constraint AS constraint_row
			WHERE constraint_row.conrelid='workspace_mutation_operations'::regclass
			  AND constraint_row.conname='workspace_mutation_project_location_valid'
		)
		SELECT function_authority.exact AND trigger_authority.exact AND
		       column_authority.exact AND constraint_authority.exact
		FROM function_authority,trigger_authority,column_authority,constraint_authority
	`, workspaceMutationProjectLocationInsertFunctionSHA256,
		workspaceMutationProjectLocationCurrentFunctionSHA256,
		workspaceMutationProjectLocationUpdateFunctionSHA256,
		workspaceMutationProjectLocationImmutableFunctionSHA256,
		workspaceMutationProjectLocationChangeGuardSHA256,
		workspaceMutationProjectLocationConstraintSHA256,
	).Scan(&databaseAuthority)
	if err != nil {
		return fmt.Errorf("inspect workspace mutation project-location database authority: %w", err)
	}
	if !databaseAuthority {
		return fmt.Errorf("workspace mutation project-location database authority differs from the registered contract")
	}

	var operationID string
	err = r.pool.QueryRow(ctx, `
		SELECT operation.id
		FROM workspace_mutation_operations AS operation
		LEFT JOIN jobs ON jobs.id=operation.job_id
		LEFT JOIN projects ON projects.id=operation.project_id
		WHERE operation.status NOT IN ($1,$2) AND (
			jobs.id IS NULL OR projects.id IS NULL OR
			jobs.project_id IS DISTINCT FROM operation.project_id OR
			projects.location IS DISTINCT FROM operation.project_location
		)
		ORDER BY operation.id
		LIMIT 1
	`, workspaceMutationVerified, workspaceMutationVerificationFailed).Scan(&operationID)
	if err == nil {
		return fmt.Errorf(
			"stale nonterminal workspace mutation %s differs from current project-location authority",
			operationID,
		)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("validate workspace mutation project-location state: %w", err)
	}
	return nil
}
