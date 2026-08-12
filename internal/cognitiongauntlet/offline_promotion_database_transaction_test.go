package cognitiongauntlet

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOfflineDatabaseAuthorityCreationStartsProcessRolesDisabled(t *testing.T) {
	admin := offlineAuthorityTestPool(t)
	identity := func(prefix string) string {
		value, err := randomProcessIdentity(prefix)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	runtimeSchema := identity("disabled_runtime_")
	hostSchema := identity("disabled_host_")
	inferenceRole := identity("disabled_inference_")
	hostRole := identity("disabled_host_role_")
	t.Cleanup(func() {
		dropOfflineAuthorityFixture(admin, runtimeSchema, hostSchema, inferenceRole)
		dropOfflineAuthorityFixture(admin, "", "", hostRole)
	})
	if err := createOfflineDatabaseAuthorities(
		t.Context(), admin, runtimeSchema, hostSchema,
		inferenceRole, "inference-password", hostRole, "host-password",
	); err != nil {
		t.Fatal(err)
	}

	for _, role := range []string{inferenceRole, hostRole} {
		var login, superuser, createDB, createRole, inherit, replication, bypassRLS bool
		var connectionLimit int
		if err := admin.QueryRow(t.Context(), `
			SELECT rolcanlogin,rolsuper,rolcreatedb,rolcreaterole,rolinherit,
			       rolreplication,rolbypassrls,rolconnlimit
			FROM pg_roles WHERE rolname=$1
		`, role).Scan(
			&login, &superuser, &createDB, &createRole, &inherit,
			&replication, &bypassRLS, &connectionLimit,
		); err != nil {
			t.Fatal(err)
		}
		if login || superuser || createDB || createRole || inherit || replication || bypassRLS ||
			connectionLimit != 8 {
			t.Fatalf(
				"process role %q authority=(login=%t super=%t createdb=%t createrole=%t inherit=%t replication=%t bypassrls=%t limit=%d)",
				role, login, superuser, createDB, createRole, inherit, replication, bypassRLS,
				connectionLimit,
			)
		}
	}
}

func TestOfflineDatabaseAuthorityCreationRollsBackLateFailure(t *testing.T) {
	admin := offlineAuthorityTestPool(t)
	runtimeSchema, _ := randomProcessIdentity("rollback_runtime_")
	hostSchema, _ := randomProcessIdentity("rollback_host_")
	role, _ := randomProcessIdentity("rollback_role_")
	t.Cleanup(func() { dropOfflineAuthorityFixture(admin, runtimeSchema, hostSchema, role) })
	err := createOfflineDatabaseAuthorities(
		t.Context(), admin, runtimeSchema, hostSchema,
		role, "inference-password", role, "host-password",
	)
	if err == nil {
		t.Fatal("duplicate final role creation unexpectedly succeeded")
	}
	for _, schema := range []string{runtimeSchema, hostSchema} {
		var exists bool
		if queryErr := admin.QueryRow(t.Context(),
			`SELECT to_regnamespace($1) IS NOT NULL`, schema,
		).Scan(&exists); queryErr != nil || exists {
			t.Fatalf("rolled-back schema %q exists=%t error=%v", schema, exists, queryErr)
		}
	}
	var roleExists bool
	if queryErr := admin.QueryRow(t.Context(),
		`SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)`, role,
	).Scan(&roleExists); queryErr != nil || roleExists {
		t.Fatalf("rolled-back role exists=%t error=%v", roleExists, queryErr)
	}
}

func TestOfflineAuthorityTransactionRollsBackSecondGrantFailure(t *testing.T) {
	admin := offlineAuthorityTestPool(t)
	schema, _ := randomProcessIdentity("rollback_grant_")
	role, _ := randomProcessIdentity("rollback_grantee_")
	dropOfflineAuthorityFixture(admin, schema, "", role)
	createOfflineGrantFixture(t, admin, schema, role, false)
	t.Cleanup(func() { dropOfflineAuthorityFixture(admin, schema, "", role) })
	schemaID, roleID := pgx.Identifier{schema}.Sanitize(), pgx.Identifier{role}.Sanitize()
	err := executeOfflineAuthorityTransaction(
		t.Context(), admin, "forced second grant", []string{
			"GRANT USAGE ON SCHEMA " + schemaID + " TO " + roleID,
			"GRANT SELECT ON ALL TABLES IN SCHEMA " + schemaID +
				" TO " + pgx.Identifier{"absent_grantee"}.Sanitize(),
		}, func(context.Context, pgx.Tx) error { return nil },
	)
	if err == nil {
		t.Fatal("forced second GRANT failure unexpectedly committed")
	}
	assertOfflinePrivilege(t, admin, role, schema, "USAGE", false)
}

func TestOfflineGrantSetsRollbackWhenExactPrivilegeValidationFails(t *testing.T) {
	for _, runtime := range []bool{false, true} {
		name := "host"
		if runtime {
			name = "runtime"
		}
		t.Run(name, func(t *testing.T) {
			admin := offlineAuthorityTestPool(t)
			schema, _ := randomProcessIdentity("rollback_" + name + "_")
			role, _ := randomProcessIdentity("rollback_" + name + "_role_")
			createOfflineGrantFixture(t, admin, schema, role, runtime)
			t.Cleanup(func() { dropOfflineAuthorityFixture(admin, schema, "", role) })
			schemaID, roleID := pgx.Identifier{schema}.Sanitize(), pgx.Identifier{role}.Sanitize()
			if _, err := admin.Exec(t.Context(), "GRANT CREATE ON SCHEMA "+schemaID+" TO "+roleID); err != nil {
				t.Fatal(err)
			}
			var err error
			if runtime {
				err = grantOfflineRuntimeAuthority(t.Context(), admin, schema, role)
			} else {
				err = grantOfflineHostAuthority(t.Context(), admin, schema, role)
			}
			if err == nil {
				t.Fatal("grant set with preexisting excess authority committed")
			}
			assertOfflinePrivilege(t, admin, role, schema, "USAGE", false)
			assertOfflinePrivilege(t, admin, role, schema, "CREATE", true)
			var tableSelect bool
			if err := admin.QueryRow(t.Context(), `SELECT has_table_privilege($1,$2,'SELECT')`,
				role, schema+".authority_probe",
			).Scan(&tableSelect); err != nil || tableSelect {
				t.Fatalf("rolled-back table grant=%t error=%v", tableSelect, err)
			}
		})
	}
}

func TestOfflineGrantSetsCommitOnlyExactRegisteredAuthority(t *testing.T) {
	for _, runtime := range []bool{false, true} {
		name := "host"
		if runtime {
			name = "runtime"
		}
		t.Run(name, func(t *testing.T) {
			admin := offlineAuthorityTestPool(t)
			schema, _ := randomProcessIdentity("exact_" + name + "_")
			role, _ := randomProcessIdentity("exact_" + name + "_role_")
			createOfflineGrantFixture(t, admin, schema, role, runtime)
			t.Cleanup(func() { dropOfflineAuthorityFixture(admin, schema, "", role) })
			var err error
			if runtime {
				err = grantOfflineRuntimeAuthority(t.Context(), admin, schema, role)
			} else {
				err = grantOfflineHostAuthority(t.Context(), admin, schema, role)
			}
			if err != nil {
				t.Fatal(err)
			}
			assertOfflinePrivilege(t, admin, role, schema, "USAGE", true)
			assertOfflinePrivilege(t, admin, role, schema, "CREATE", false)
			var tableSelect bool
			if err := admin.QueryRow(t.Context(), `SELECT has_table_privilege($1,$2,'SELECT')`,
				role, schema+".authority_probe",
			).Scan(&tableSelect); err != nil || !tableSelect {
				t.Fatalf("exact table grant=%t error=%v", tableSelect, err)
			}
			if runtime {
				var sequenceUsage bool
				if err := admin.QueryRow(t.Context(), `SELECT has_sequence_privilege($1,$2,'USAGE')`,
					role, schema+".authority_probe_sequence",
				).Scan(&sequenceUsage); err != nil || !sequenceUsage {
					t.Fatalf("exact sequence grant=%t error=%v", sequenceUsage, err)
				}
			}
		})
	}
}

func offlineAuthorityTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("OMNI_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run offline authority transaction tests")
	}
	pool, err := pgxpool.New(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
