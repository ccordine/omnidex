package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresJobExecutionIdentityIsImmutable(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "169"),
	); err != nil {
		t.Fatal(err)
	}
	fixture := newWorkspaceMutationPipelineActionFixture(
		t, repository, model.PipelineCoding, "immutable-identity",
	)
	if _, err := repository.prepareWorkspaceMutation(
		t.Context(), fixture.claim.Authority, fixture.command, fixture.identity,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE jobs SET pipeline='scrum' WHERE id=$1
	`, fixture.claim.Job.ID); err == nil ||
		!strings.Contains(err.Error(), "job pipeline identity is immutable") {
		t.Fatalf("pipeline identity rewrite error=%v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE job_steps SET action='objective_resolve' WHERE id=$1
	`, fixture.claim.Step.ID); err == nil ||
		!strings.Contains(err.Error(), "job step action identity is immutable") {
		t.Fatalf("step action identity rewrite error=%v", err)
	}
	if err := repository.markWorkspaceMutationApplying(
		t.Context(), fixture.claim.Authority, fixture.command, fixture.identity,
	); err != nil {
		t.Fatalf("valid operation lost authority after rejected identity rewrites: %v", err)
	}
	if err := repository.ValidateRuntimeAuthority(t.Context()); err != nil {
		t.Fatalf("validate immutable execution identity authority: %v", err)
	}
	assertAppliedMigrationCount(t, pool, jobExecutionIdentityImmutabilityMigration, 1)
}

func TestPostgresJobExecutionIdentityRejectsStaleOperationAtomically(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "168"),
	); err != nil {
		t.Fatal(err)
	}
	fixture := newWorkspaceMutationPipelineActionFixture(
		t, repository, model.PipelineCoding, "identity-stale-row",
	)
	if _, err := repository.prepareWorkspaceMutation(
		t.Context(), fixture.claim.Authority, fixture.command, fixture.identity,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE job_steps SET action='objective_resolve' WHERE id=$1
	`, fixture.claim.Step.ID); err != nil {
		t.Fatal(err)
	}
	before := jobExecutionIdentityCatalog(t, pool)
	err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "169"),
	)
	if err == nil || !strings.Contains(
		err.Error(), "job execution identity found stale workspace mutation",
	) {
		t.Fatalf("stale execution identity cutover error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, jobExecutionIdentityImmutabilityMigration, 0)
	if after := jobExecutionIdentityCatalog(t, pool); after != before {
		t.Fatalf("stale operation rejection changed identity catalog: before=%s after=%s", before, after)
	}
}

func TestPostgresJobExecutionIdentityRejectsChangedPriorAtomically(t *testing.T) {
	tests := []struct {
		name, tamper, want string
	}{
		{
			name: "pipeline function",
			tamper: `
				CREATE OR REPLACE FUNCTION enforce_jobs_executable_pipeline_authority()
				RETURNS TRIGGER AS $$ BEGIN RETURN NEW; END $$ LANGUAGE plpgsql
			`,
			want: "exact prior pipeline guard",
		},
		{
			name:   "pipeline trigger",
			tamper: `ALTER TABLE jobs DISABLE TRIGGER jobs_executable_pipeline_authority`,
			want:   "exact prior pipeline guard",
		},
		{
			name: "step function",
			tamper: `
				CREATE OR REPLACE FUNCTION prevent_job_step_generation_identity_mutation()
				RETURNS TRIGGER AS $$ BEGIN RETURN NEW; END $$ LANGUAGE plpgsql
			`,
			want: "exact prior step guard",
		},
		{
			name:   "step trigger",
			tamper: `ALTER TABLE job_steps DISABLE TRIGGER job_steps_generation_identity_immutable`,
			want:   "exact prior step guard",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pool := openIsolatedMigrationPool(t)
			repository := New(pool)
			if err := repository.EnsureSchema(
				t.Context(), loadMigrationBundleThroughPrefix(t, "168"),
			); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(t.Context(), test.tamper); err != nil {
				t.Fatal(err)
			}
			before := jobExecutionIdentityCatalog(t, pool)
			err := repository.EnsureSchema(
				t.Context(), loadMigrationBundleThroughPrefix(t, "169"),
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("changed prior identity authority error=%v", err)
			}
			assertAppliedMigrationCount(
				t, pool, jobExecutionIdentityImmutabilityMigration, 0,
			)
			if after := jobExecutionIdentityCatalog(t, pool); after != before {
				t.Fatalf("changed-prior rejection mutated catalog: before=%s after=%s", before, after)
			}
		})
	}
}

func TestPostgresStartupRejectsTamperedJobStepExecutionIdentityAuthority(t *testing.T) {
	tests := []struct {
		name, tamper string
	}{
		{
			name: "function",
			tamper: `
				CREATE OR REPLACE FUNCTION prevent_job_step_generation_identity_mutation()
				RETURNS TRIGGER AS $$ BEGIN RETURN NEW; END $$ LANGUAGE plpgsql
			`,
		},
		{
			name:   "disabled trigger",
			tamper: `ALTER TABLE job_steps DISABLE TRIGGER job_steps_generation_identity_immutable`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pool := openIsolatedMigrationPool(t)
			repository := New(pool)
			if err := repository.EnsureSchema(
				t.Context(), loadMigrationBundleThroughPrefix(t, "169"),
			); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(t.Context(), test.tamper); err != nil {
				t.Fatal(err)
			}
			if err := repository.ValidateRuntimeAuthority(t.Context()); err == nil ||
				!strings.Contains(err.Error(), "job step execution identity database authority") {
				t.Fatalf("tampered step identity startup error=%v", err)
			}
		})
	}
}

func jobExecutionIdentityCatalog(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var catalog string
	if err := pool.QueryRow(t.Context(), `
		SELECT jsonb_build_object(
			'jobs_source',(
				SELECT prosrc FROM pg_proc
				WHERE oid='enforce_jobs_executable_pipeline_authority()'::regprocedure
			),
			'steps_source',(
				SELECT prosrc FROM pg_proc
				WHERE oid='prevent_job_step_generation_identity_mutation()'::regprocedure
			),
			'triggers',(
				SELECT jsonb_agg(
					jsonb_build_array(tgrelid::regclass::text,tgname,tgenabled)
					ORDER BY tgrelid::regclass::text,tgname
				)
				FROM pg_trigger
				WHERE tgname IN (
					'jobs_executable_pipeline_authority',
					'jobs_history_truncate_immutable',
					'job_steps_generation_identity_immutable'
				)
			)
		)::text
	`).Scan(&catalog); err != nil {
		t.Fatal(err)
	}
	return catalog
}
