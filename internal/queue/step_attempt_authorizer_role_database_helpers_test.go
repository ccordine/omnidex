package queue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const restrictedAttemptRolePassword = "omnidex-test-fence-password"

type stepAttemptFenceFixture struct {
	Context    context.Context
	Pool       *pgxpool.Pool
	Repository *Repository
	Authority  model.StepAttemptAuthority
}

func startStepAttemptFenceFixture(t *testing.T, label string) stepAttemptFenceFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "059")); err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(
		ctx, label, model.PipelineAssistant, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, label+"-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("step-attempt fence fixture claim=%+v", claim)
	}
	return stepAttemptFenceFixture{
		Context: ctx, Pool: pool, Repository: repository, Authority: claim.Authority,
	}
}

func provisionRestrictedAttemptRole(
	t *testing.T,
	fixture stepAttemptFenceFixture,
) (*pgxpool.Pool, string, string) {
	t.Helper()
	role := fmt.Sprintf("omni_host_%d", time.Now().UnixNano())
	createRestrictedDatabaseRole(t, fixture, role)
	if err := fixture.Repository.ProvisionStepAttemptAuthorizerRole(
		fixture.Context, role,
	); err != nil {
		t.Fatal(err)
	}
	var runtimeSchema string
	if err := fixture.Pool.QueryRow(fixture.Context, `SELECT current_schema()`).Scan(&runtimeSchema); err != nil {
		t.Fatal(err)
	}
	hostSchema := role + "_surface"
	if _, err := fixture.Pool.Exec(fixture.Context,
		`CREATE SCHEMA `+pgx.Identifier{hostSchema}.Sanitize()+
			` AUTHORIZATION `+pgx.Identifier{role}.Sanitize(),
	); err != nil {
		t.Fatal(err)
	}

	config := fixture.Pool.Config().Copy()
	config.ConnConfig.User = role
	config.ConnConfig.Password = restrictedAttemptRolePassword
	config.ConnConfig.RuntimeParams["search_path"] = hostSchema + "," + runtimeSchema + ",public"
	restricted, err := pgxpool.NewWithConfig(fixture.Context, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		restricted.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := fixture.Pool.Exec(ctx,
			`DROP SCHEMA `+pgx.Identifier{hostSchema}.Sanitize()+` CASCADE`,
		); err != nil {
			t.Errorf("drop restricted host schema: %v", err)
		}
		if _, err := fixture.Pool.Exec(ctx,
			`DROP OWNED BY `+pgx.Identifier{role}.Sanitize(),
		); err != nil {
			t.Errorf("drop restricted host grants: %v", err)
		}
		if _, err := fixture.Pool.Exec(ctx,
			`DROP ROLE IF EXISTS `+pgx.Identifier{role}.Sanitize(),
		); err != nil {
			t.Errorf("drop restricted host role: %v", err)
		}
	})
	return restricted, role, runtimeSchema
}

func createRestrictedDatabaseRole(
	t *testing.T,
	fixture stepAttemptFenceFixture,
	role string,
) {
	t.Helper()
	createRestrictedDatabaseRoleWithLogin(t, fixture, role, true)
}

func createRestrictedDatabaseRoleWithLogin(
	t *testing.T,
	fixture stepAttemptFenceFixture,
	role string,
	canLogin bool,
) {
	t.Helper()
	login := "NOLOGIN"
	if canLogin {
		login = "LOGIN"
	}
	statement := `CREATE ROLE ` + pgx.Identifier{role}.Sanitize() +
		` ` + login + ` NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS` +
		` PASSWORD '` + restrictedAttemptRolePassword + `'`
	if _, err := fixture.Pool.Exec(fixture.Context, statement); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = fixture.Pool.Exec(ctx, `REASSIGN OWNED BY `+pgx.Identifier{role}.Sanitize()+` TO CURRENT_USER`)
		_, _ = fixture.Pool.Exec(ctx, `DROP OWNED BY `+pgx.Identifier{role}.Sanitize())
		_, _ = fixture.Pool.Exec(ctx, `DROP ROLE IF EXISTS `+pgx.Identifier{role}.Sanitize())
	})
}

func hostFenceAuthoritySchema(
	t *testing.T,
	fixture stepAttemptFenceFixture,
) string {
	t.Helper()
	var schema string
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT authority_schema FROM step_attempt_transaction_fence_authority
		WHERE singleton
	`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	return schema
}
