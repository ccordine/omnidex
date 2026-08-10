package queue

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresRestrictedHostRoleHasOnlyTransactionalFenceAuthority(t *testing.T) {
	fixture := startStepAttemptFenceFixture(t, "restricted-attempt-authorizer")
	restricted, role, runtimeSchema := provisionRestrictedAttemptRole(t, fixture)
	repository := New(restricted)

	var currentUser string
	if err := restricted.QueryRow(fixture.Context, `SELECT current_user`).Scan(&currentUser); err != nil {
		t.Fatal(err)
	}
	if currentUser != role {
		t.Fatalf("restricted current_user=%q want %q", currentUser, role)
	}
	var runtimeUsage, runtimeCreate, authorityCreate bool
	var executableAuthorityFunctions int
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT has_schema_privilege($1,$2,'USAGE'),
		       has_schema_privilege($1,$2,'CREATE'),
		       has_schema_privilege($1,$3,'CREATE'),
		       (SELECT COUNT(*) FROM pg_proc procedures
		        JOIN pg_namespace namespaces ON namespaces.oid=procedures.pronamespace
		        WHERE namespaces.nspname=$3 AND
		              has_function_privilege($1,procedures.oid,'EXECUTE'))
	`, role, runtimeSchema, hostFenceAuthoritySchema(t, fixture)).Scan(
		&runtimeUsage, &runtimeCreate, &authorityCreate, &executableAuthorityFunctions,
	); err != nil {
		t.Fatal(err)
	}
	if runtimeUsage || runtimeCreate || authorityCreate || executableAuthorityFunctions != 1 {
		t.Fatalf(
			"host privileges runtime_usage=%t runtime_create=%t authority_create=%t executable_functions=%d",
			runtimeUsage, runtimeCreate, authorityCreate, executableAuthorityFunctions,
		)
	}
	for _, statement := range []string{
		`SELECT id FROM ` + pgx.Identifier{runtimeSchema, "jobs"}.Sanitize() + ` LIMIT 1`,
		`UPDATE ` + pgx.Identifier{runtimeSchema, "jobs"}.Sanitize() + ` SET status=status`,
		`SELECT nextval('` + runtimeSchema + `.jobs_id_seq')`,
	} {
		if _, err := restricted.Exec(fixture.Context, statement); err == nil {
			t.Fatalf("restricted host role executed forbidden queue DML: %s", statement)
		}
	}

	tx, err := restricted.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.Context)
	if err := repository.AuthorizeStepAttemptTransaction(
		fixture.Context, tx, fixture.Authority,
	); err != nil {
		t.Fatalf("restricted transactional fence rejected current authority: %v", err)
	}
}

func TestPostgresProvisionAcceptsDisabledRoleWithoutGrantingLoginAuthority(t *testing.T) {
	fixture := startStepAttemptFenceFixture(t, "disabled-attempt-authorizer")
	role := fmt.Sprintf("omni_host_disabled_%d", time.Now().UnixNano())
	createRestrictedDatabaseRoleWithLogin(t, fixture, role, false)
	if err := fixture.Repository.ProvisionStepAttemptAuthorizerRole(
		fixture.Context, role,
	); err != nil {
		t.Fatalf("provision disabled host role: %v", err)
	}

	config := fixture.Pool.Config().Copy()
	config.ConnConfig.User = role
	config.ConnConfig.Password = restrictedAttemptRolePassword
	config.ConnConfig.RuntimeParams["search_path"] = "public"
	assertRestrictedRoleCannotConnect(t, fixture, config)

	roleIdentifier := pgx.Identifier{role}.Sanitize()
	if _, err := fixture.Pool.Exec(fixture.Context, `ALTER ROLE `+roleIdentifier+` LOGIN`); err != nil {
		t.Fatal(err)
	}
	restricted, err := pgxpool.NewWithConfig(fixture.Context, config.Copy())
	if err != nil {
		t.Fatal(err)
	}
	if err := restricted.Ping(fixture.Context); err != nil {
		restricted.Close()
		t.Fatalf("enabled restricted role cannot connect: %v", err)
	}
	tx, err := restricted.Begin(fixture.Context)
	if err != nil {
		restricted.Close()
		t.Fatal(err)
	}
	if err := New(restricted).AuthorizeStepAttemptTransaction(
		fixture.Context, tx, fixture.Authority,
	); err != nil {
		_ = tx.Rollback(fixture.Context)
		restricted.Close()
		t.Fatalf("enabled restricted role cannot execute exact fence: %v", err)
	}
	if err := tx.Rollback(fixture.Context); err != nil {
		restricted.Close()
		t.Fatal(err)
	}
	var runtimeSchema string
	if err := fixture.Pool.QueryRow(fixture.Context, `SELECT current_schema()`).Scan(&runtimeSchema); err != nil {
		restricted.Close()
		t.Fatal(err)
	}
	for label, statement := range map[string]string{
		"queue table": `SELECT id FROM ` +
			pgx.Identifier{runtimeSchema, "jobs"}.Sanitize() + ` LIMIT 1`,
		"unrelated runtime function": `SELECT ` +
			pgx.Identifier{runtimeSchema, "cognition_canonical_jsonb"}.Sanitize() + `('{}'::jsonb)`,
	} {
		if _, err := restricted.Exec(fixture.Context, statement); err == nil {
			restricted.Close()
			t.Fatalf("enabled restricted role acquired %s authority", label)
		}
	}
	restricted.Close()

	if _, err := fixture.Pool.Exec(fixture.Context, `ALTER ROLE `+roleIdentifier+` NOLOGIN`); err != nil {
		t.Fatal(err)
	}
	assertRestrictedRoleCannotConnect(t, fixture, config)
}

func assertRestrictedRoleCannotConnect(
	t *testing.T,
	fixture stepAttemptFenceFixture,
	config *pgxpool.Config,
) {
	t.Helper()
	pool, err := pgxpool.NewWithConfig(fixture.Context, config.Copy())
	if err != nil {
		return
	}
	defer pool.Close()
	if err := pool.Ping(fixture.Context); err == nil {
		t.Fatal("disabled restricted role connected to PostgreSQL")
	}
}

func TestPostgresTransactionalFenceIgnoresHostSchemaAndTemporaryShadows(t *testing.T) {
	fixture := startStepAttemptFenceFixture(t, "shadow-attempt-authorizer")
	restricted, _, _ := provisionRestrictedAttemptRole(t, fixture)
	if _, err := restricted.Exec(fixture.Context, `
		CREATE TEMP TABLE jobs (id BIGINT,status TEXT,current_generation BIGINT);
		CREATE TEMP TABLE job_steps (job_id BIGINT,id BIGINT,status TEXT);
		CREATE TEMP TABLE job_step_attempts (job_id BIGINT,status TEXT);
		CREATE FUNCTION omnidex_authorize_step_attempt_transaction_v1(
			BIGINT,BIGINT,BIGINT,BIGINT,TEXT
		) RETURNS BOOLEAN LANGUAGE SQL SECURITY DEFINER AS 'SELECT FALSE';
	`); err != nil {
		t.Fatal(err)
	}
	tx, err := restricted.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(fixture.Context)
	if err := New(restricted).AuthorizeStepAttemptTransaction(
		fixture.Context, tx, fixture.Authority,
	); err != nil {
		t.Fatalf("runtime fence was intercepted by a host shadow: %v", err)
	}
}

func TestPostgresTransactionalFenceIsNotExecutableByPublic(t *testing.T) {
	fixture := startStepAttemptFenceFixture(t, "public-attempt-authorizer")
	authoritySchema := hostFenceAuthoritySchema(t, fixture)
	var executable bool
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT EXISTS (
			SELECT 1 FROM aclexplode(
				COALESCE(procedures.proacl,acldefault('f',procedures.proowner))
			) privileges
			WHERE privileges.grantee=0 AND privileges.privilege_type='EXECUTE'
		)
		FROM pg_proc procedures
		JOIN pg_namespace namespaces ON namespaces.oid=procedures.pronamespace
		WHERE namespaces.nspname=$1 AND procedures.proname=$2
		  AND pg_catalog.oidvectortypes(procedures.proargtypes)=
		      'bigint, bigint, bigint, bigint, text'
	`, authoritySchema, stepAttemptFenceFunction).Scan(&executable); err != nil {
		t.Fatal(err)
	}
	if executable {
		t.Fatal("transactional step-attempt fence remains executable by PUBLIC")
	}
}

func TestPostgresProvisionRejectsPrivilegedOrDataAuthorizedRole(t *testing.T) {
	fixture := startStepAttemptFenceFixture(t, "reject-attempt-authorizer")
	role := fmt.Sprintf("omni_host_reject_%d", time.Now().UnixNano())
	createRestrictedDatabaseRole(t, fixture, role)
	if _, err := fixture.Pool.Exec(fixture.Context,
		`GRANT SELECT ON jobs TO `+pgx.Identifier{role}.Sanitize(),
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.Repository.ProvisionStepAttemptAuthorizerRole(
		fixture.Context, role,
	); err == nil || !strings.Contains(err.Error(), "forbidden queue data privileges") {
		t.Fatalf("ProvisionStepAttemptAuthorizerRole error=%v", err)
	}
}

func TestPostgresProvisionRejectsExecutableFenceNameOverload(t *testing.T) {
	fixture := startStepAttemptFenceFixture(t, "reject-attempt-overload")
	authoritySchema := hostFenceAuthoritySchema(t, fixture)
	overload := pgx.Identifier{authoritySchema, stepAttemptFenceFunction}.Sanitize()
	if _, err := fixture.Pool.Exec(fixture.Context,
		`CREATE FUNCTION `+overload+`(text) RETURNS boolean LANGUAGE SQL AS 'SELECT TRUE'`,
	); err != nil {
		t.Fatal(err)
	}
	role := fmt.Sprintf("omni_host_overload_%d", time.Now().UnixNano())
	createRestrictedDatabaseRole(t, fixture, role)
	if err := fixture.Repository.ProvisionStepAttemptAuthorizerRole(
		fixture.Context, role,
	); err == nil || !strings.Contains(err.Error(), "another authority function") {
		t.Fatalf("ProvisionStepAttemptAuthorizerRole error=%v", err)
	}
}
