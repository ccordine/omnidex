package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresWorkspaceMutationPipelineActionAuthorityAcceptsRegisteredPairs(
	t *testing.T,
) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "168"),
	); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, workspaceMutationPipelineActionAuthorityMigration, 1)
	if digest := loadWorkspaceMutationInsertSourceSHA256(t, pool); digest != workspaceMutationInsertSourceSHA256 {
		t.Fatalf("workspace mutation insert source=%s want %s", digest, workspaceMutationInsertSourceSHA256)
	}
	if digest := loadWorkspaceMutationCurrentSourceSHA256(t, pool); digest != workspaceMutationCurrentSourceSHA256 {
		t.Fatalf("workspace mutation current source=%s want %s", digest, workspaceMutationCurrentSourceSHA256)
	}

	for _, pipeline := range []string{
		model.PipelineChat, model.PipelineCoding, model.PipelineScrum,
	} {
		t.Run(pipeline, func(t *testing.T) {
			fixture := newWorkspaceMutationPipelineActionFixture(
				t, repository, pipeline,
				workspaceMutationPipelineActionLabel(pipeline, "positive"),
			)
			record, err := repository.prepareWorkspaceMutation(
				t.Context(), fixture.claim.Authority, fixture.command, fixture.identity,
			)
			if err != nil {
				t.Fatal(err)
			}
			if record.ID != fixture.identity.ID || record.Status != workspaceMutationPrepared {
				t.Fatalf("prepared workspace mutation=%+v", record)
			}
		})
	}
}

func TestPostgresWorkspaceMutationPipelineActionAuthorityRejectsChangedPairOnTransition(
	t *testing.T,
) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "168"),
	); err != nil {
		t.Fatal(err)
	}
	fixture := newWorkspaceMutationPipelineActionFixture(
		t, repository, model.PipelineCoding, "transition-mismatch",
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
	err := repository.markWorkspaceMutationApplying(
		t.Context(), fixture.claim.Authority, fixture.command, fixture.identity,
	)
	if err == nil || !strings.Contains(
		err.Error(), "lost exact current step-attempt authority",
	) {
		t.Fatalf("changed transition pair error=%v", err)
	}
}

func TestPostgresWorkspaceMutationPipelineActionAuthorityRejectsStaleRowsAtomically(
	t *testing.T,
) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "167"),
	); err != nil {
		t.Fatal(err)
	}
	fixture := newWorkspaceMutationPipelineActionFixture(
		t, repository, model.PipelineCoding, "stale-row",
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
	beforeInsert := workspaceMutationInsertAuthorityCatalog(t, pool)
	beforeCurrent := loadWorkspaceMutationCurrentSourceSHA256(t, pool)
	err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "168"),
	)
	if err == nil || !strings.Contains(err.Error(), "has stale pipeline/action authority") {
		t.Fatalf("stale workspace mutation cutover error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, workspaceMutationPipelineActionAuthorityMigration, 0)
	if after := workspaceMutationInsertAuthorityCatalog(t, pool); after != beforeInsert {
		t.Fatal("stale-row rejection changed insert authority")
	}
	if after := loadWorkspaceMutationCurrentSourceSHA256(t, pool); after != beforeCurrent {
		t.Fatal("stale-row rejection changed transition authority")
	}
}

func TestPostgresWorkspaceMutationPipelineActionAuthorityRejectsMismatchedPairs(
	t *testing.T,
) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "168"),
	); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		pipeline, mismatchedAction string
	}{
		{pipeline: model.PipelineChat, mismatchedAction: "v3_coding"},
		{pipeline: model.PipelineCoding, mismatchedAction: "objective_resolve"},
		{pipeline: model.PipelineScrum, mismatchedAction: "objective_resolve"},
	}
	for _, test := range tests {
		t.Run(test.pipeline, func(t *testing.T) {
			fixture := newWorkspaceMutationPipelineActionFixture(
				t, repository, test.pipeline,
				workspaceMutationPipelineActionLabel(test.pipeline, "mismatch"),
			)
			if _, err := pool.Exec(t.Context(), `
				UPDATE job_steps SET action=$2 WHERE id=$1
			`, fixture.claim.Step.ID, test.mismatchedAction); err != nil {
				t.Fatal(err)
			}
			_, err := repository.prepareWorkspaceMutation(
				t.Context(), fixture.claim.Authority, fixture.command, fixture.identity,
			)
			if err == nil || !strings.Contains(
				err.Error(), "requires the exact current active step attempt and root",
			) {
				t.Fatalf("mismatched %s/%s error=%v", test.pipeline, test.mismatchedAction, err)
			}
			requireNoWorkspaceMutationOperation(t, pool, fixture.identity.ID)
		})
	}
}

func TestPostgresWorkspaceMutationPipelineActionAuthorityRejectsChangedPriorAtomically(
	t *testing.T,
) {
	tests := []struct {
		name   string
		tamper string
	}{
		{
			name: "function source",
			tamper: `
				CREATE OR REPLACE FUNCTION validate_workspace_mutation_insert()
				RETURNS TRIGGER AS $$
				BEGIN
					RETURN NEW;
				END;
				$$ LANGUAGE plpgsql
			`,
		},
		{
			name: "disabled trigger",
			tamper: `
				ALTER TABLE workspace_mutation_operations
				DISABLE TRIGGER workspace_mutation_insert_validate
			`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pool := openIsolatedMigrationPool(t)
			repository := New(pool)
			if err := repository.EnsureSchema(
				t.Context(), loadMigrationBundleThroughPrefix(t, "167"),
			); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(t.Context(), test.tamper); err != nil {
				t.Fatal(err)
			}
			before := workspaceMutationInsertAuthorityCatalog(t, pool)
			err := repository.EnsureSchema(
				t.Context(), loadMigrationBundleThroughPrefix(t, "168"),
			)
			if err == nil || !strings.Contains(
				err.Error(), "requires the exact prior insert guard",
			) {
				t.Fatalf("changed prior workspace mutation authority error=%v", err)
			}
			assertAppliedMigrationCount(t, pool, workspaceMutationPipelineActionAuthorityMigration, 0)
			if after := workspaceMutationInsertAuthorityCatalog(t, pool); after != before {
				t.Fatalf("changed-prior rejection mutated catalog: before=%s after=%s", before, after)
			}
		})
	}
}
