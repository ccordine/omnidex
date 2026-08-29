package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresWorkspaceMutationProjectLocationAuthorityMigratesExactPriorRow(
	t *testing.T,
) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "178"),
	); err != nil {
		t.Fatal(err)
	}
	fixture := newWorkspaceMutationPipelineActionFixture(
		t, repository, model.PipelineCoding, "location-179-forward",
	)
	if _, err := prepareWorkspaceMutationBeforeProjectLocation(t, fixture); err != nil {
		t.Fatal(err)
	}
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "179"),
	); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, workspaceMutationProjectLocationAuthorityMigration, 1)
	assertWorkspaceMutationProjectLocationCatalog(t, pool)

	var projectLocation, runtimeRoot, status string
	var sealed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT project_location,workspace_root,status,sealed_at IS NOT NULL
		FROM workspace_mutation_operations WHERE id=$1
	`, fixture.identity.ID).Scan(
		&projectLocation, &runtimeRoot, &status, &sealed,
	); err != nil {
		t.Fatal(err)
	}
	if projectLocation != fixture.command.ProjectLocation || runtimeRoot != fixture.command.Plan.WorkspaceRoot ||
		projectLocation != runtimeRoot || status != workspaceMutationPrepared || !sealed {
		t.Fatalf(
			"migrated mutation location/runtime/status/sealed=%q/%q/%q/%t",
			projectLocation, runtimeRoot, status, sealed,
		)
	}
}

func TestPostgresWorkspaceMutationProjectLocationAuthorityAcceptsDistinctRuntimeRoots(
	t *testing.T,
) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "179"),
	); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, workspaceMutationProjectLocationAuthorityMigration, 1)
	assertWorkspaceMutationProjectLocationCatalog(t, pool)

	for _, label := range []string{"parcel-dispatch", "room-climate"} {
		fixture := newWorkspaceMutationProjectLocationFixture(t, repository, label)
		record, err := repository.prepareWorkspaceMutation(
			t.Context(), fixture.claim.Authority, fixture.command, fixture.identity,
		)
		if err != nil {
			t.Fatalf("prepare unrelated mutation %q: %v", label, err)
		}
		if record.ID != fixture.identity.ID || record.Status != workspaceMutationPrepared {
			t.Fatalf("prepared unrelated mutation %q=%+v", label, record)
		}
		var projectLocation, runtimeRoot, status string
		var sealed bool
		if err := pool.QueryRow(t.Context(), `
			SELECT project_location,workspace_root,status,sealed_at IS NOT NULL
			FROM workspace_mutation_operations WHERE id=$1
		`, fixture.identity.ID).Scan(
			&projectLocation, &runtimeRoot, &status, &sealed,
		); err != nil {
			t.Fatal(err)
		}
		if projectLocation != fixture.projectLocation || runtimeRoot != fixture.runtimeRoot ||
			projectLocation == runtimeRoot || status != workspaceMutationPrepared || !sealed {
			t.Fatalf(
				"mutation %q location/runtime/status/sealed=%q/%q/%q/%t",
				label, projectLocation, runtimeRoot, status, sealed,
			)
		}
	}
}

func TestPostgresWorkspaceMutationProjectLocationAuthorityRejectsWrongDirectInsert(
	t *testing.T,
) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "179"),
	); err != nil {
		t.Fatal(err)
	}
	fixture := newWorkspaceMutationProjectLocationFixture(t, repository, "wrong-direct")
	wrongLocation := t.TempDir()
	if wrongLocation == fixture.projectLocation || wrongLocation == fixture.runtimeRoot {
		t.Fatal("wrong project-location fixture is not distinct")
	}
	err := insertWorkspaceMutationProjectLocationDirect(t, pool, fixture, wrongLocation)
	if err == nil || !strings.Contains(
		err.Error(), "requires the exact current active step attempt and project location",
	) {
		t.Fatalf("wrong direct project_location insert error=%v", err)
	}
	requireNoWorkspaceMutationOperation(t, pool, fixture.identity.ID)
}

func TestPostgresWorkspaceMutationProjectLocationAuthorityRejectsUpdate(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "179"),
	); err != nil {
		t.Fatal(err)
	}
	fixture := newWorkspaceMutationProjectLocationFixture(t, repository, "immutable")
	if _, err := repository.prepareWorkspaceMutation(
		t.Context(), fixture.claim.Authority, fixture.command, fixture.identity,
	); err != nil {
		t.Fatal(err)
	}
	changedLocation := fixture.projectLocation + "/moved"
	if _, err := pool.Exec(t.Context(), `
		UPDATE workspace_mutation_operations SET project_location=$2 WHERE id=$1
	`, fixture.identity.ID, changedLocation); err == nil ||
		!strings.Contains(err.Error(), "workspace mutation project location is immutable") {
		t.Fatalf("workspace project_location update error=%v", err)
	}
	var retained string
	if err := pool.QueryRow(t.Context(), `
		SELECT project_location FROM workspace_mutation_operations WHERE id=$1
	`, fixture.identity.ID).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if retained != fixture.projectLocation {
		t.Fatalf("rejected project_location update retained %q want %q", retained, fixture.projectLocation)
	}
}

func TestPostgresWorkspaceMutationProjectLocationAuthorityPreservesInsertGuards(
	t *testing.T,
) {
	t.Run("stale attempt", func(t *testing.T) {
		pool := openIsolatedMigrationPool(t)
		repository := New(pool)
		if err := repository.EnsureSchema(
			t.Context(), loadMigrationBundleThroughPrefix(t, "179"),
		); err != nil {
			t.Fatal(err)
		}
		fixture := newWorkspaceMutationProjectLocationFixture(t, repository, "stale-attempt")
		replaceStepAttemptForTest(t, pool, fixture.claim.Authority)
		err := insertWorkspaceMutationProjectLocationDirect(
			t, pool, fixture, fixture.projectLocation,
		)
		if err == nil || !strings.Contains(
			err.Error(), "requires the exact current active step attempt and project location",
		) {
			t.Fatalf("stale workspace mutation insert error=%v", err)
		}
		requireNoWorkspaceMutationOperation(t, pool, fixture.identity.ID)
	})

	t.Run("pipeline action", func(t *testing.T) {
		pool := openIsolatedMigrationPool(t)
		repository := New(pool)
		if err := repository.EnsureSchema(
			t.Context(), loadMigrationBundleThroughPrefix(t, "179"),
		); err != nil {
			t.Fatal(err)
		}
		fixture := newWorkspaceMutationProjectLocationFixture(t, repository, "pipeline-action")
		if _, err := pool.Exec(t.Context(), `
			ALTER TABLE job_steps DISABLE TRIGGER job_steps_generation_identity_immutable
		`); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(t.Context(), `
			UPDATE job_steps SET action='objective_resolve' WHERE id=$1
		`, fixture.claim.Step.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(t.Context(), `
			ALTER TABLE job_steps ENABLE TRIGGER job_steps_generation_identity_immutable
		`); err != nil {
			t.Fatal(err)
		}
		err := insertWorkspaceMutationProjectLocationDirect(
			t, pool, fixture, fixture.projectLocation,
		)
		if err == nil || !strings.Contains(
			err.Error(), "requires the exact current active step attempt and project location",
		) {
			t.Fatalf("mismatched pipeline/action workspace mutation insert error=%v", err)
		}
		requireNoWorkspaceMutationOperation(t, pool, fixture.identity.ID)
	})
}
