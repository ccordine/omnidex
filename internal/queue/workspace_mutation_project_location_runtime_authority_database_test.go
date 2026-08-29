package queue

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const workspaceMutationProjectLocationDatabaseAuthorityError = "workspace mutation project-location database authority"

func TestPostgresStartupAcceptsWorkspaceMutationProjectLocationAuthority(t *testing.T) {
	repository, _ := newWorkspaceMutationProjectLocationRuntimeAuthorityRepository(t)
	if err := repository.ValidateRuntimeAuthority(t.Context()); err != nil {
		t.Fatalf("validate exact workspace project-location authority: %v", err)
	}
}

func TestPostgresStartupRejectsTamperedWorkspaceMutationProjectLocationAuthority(
	t *testing.T,
) {
	tests := []struct {
		name   string
		tamper string
	}{
		{
			name: "insert function",
			tamper: `
				CREATE OR REPLACE FUNCTION validate_workspace_mutation_insert()
				RETURNS TRIGGER AS $$ BEGIN RETURN NEW; END $$ LANGUAGE plpgsql
			`,
		},
		{
			name: "current authority function",
			tamper: `
				CREATE OR REPLACE FUNCTION workspace_mutation_current_authority_valid(
					operation workspace_mutation_operations
				) RETURNS BOOLEAN AS $$ SELECT TRUE $$ LANGUAGE SQL
			`,
		},
		{
			name: "immutability function",
			tamper: `
				CREATE OR REPLACE FUNCTION prevent_workspace_mutation_project_location_change()
				RETURNS TRIGGER AS $$ BEGIN RETURN NEW; END $$ LANGUAGE plpgsql
			`,
		},
		{
			name: "transition function",
			tamper: `
				CREATE OR REPLACE FUNCTION validate_workspace_mutation_update()
				RETURNS TRIGGER AS $$ BEGIN RETURN NEW; END $$ LANGUAGE plpgsql
			`,
		},
		{
			name: "active-work project guard function",
			tamper: `
				CREATE OR REPLACE FUNCTION prevent_project_location_change_during_active_work()
				RETURNS TRIGGER AS $$ BEGIN RETURN NEW; END $$ LANGUAGE plpgsql
			`,
		},
		{
			name: "disabled trigger",
			tamper: `
				ALTER TABLE workspace_mutation_operations
				DISABLE TRIGGER workspace_mutation_insert_validate
			`,
		},
		{
			name: "replaced trigger",
			tamper: `
				DROP TRIGGER workspace_mutation_project_location_immutable
				ON workspace_mutation_operations;
				CREATE TRIGGER workspace_mutation_project_location_immutable
				BEFORE UPDATE ON workspace_mutation_operations
				FOR EACH ROW EXECUTE FUNCTION validate_workspace_mutation_update()
			`,
		},
		{
			name: "disabled active-work project guard trigger",
			tamper: `
				ALTER TABLE projects
				DISABLE TRIGGER projects_active_work_location_guard
			`,
		},
		{
			name: "replaced active-work project guard trigger",
			tamper: `
				DROP TRIGGER projects_active_work_location_guard ON projects;
				CREATE TRIGGER projects_active_work_location_guard
				BEFORE UPDATE ON projects
				FOR EACH ROW EXECUTE FUNCTION prevent_workspace_mutation_project_location_change()
			`,
		},
		{
			name: "dropped constraint",
			tamper: `
				ALTER TABLE workspace_mutation_operations
				DROP CONSTRAINT workspace_mutation_project_location_valid
			`,
		},
		{
			name: "wrong constraint",
			tamper: `
				ALTER TABLE workspace_mutation_operations
				DROP CONSTRAINT workspace_mutation_project_location_valid;
				ALTER TABLE workspace_mutation_operations
				ADD CONSTRAINT workspace_mutation_project_location_valid
				CHECK (project_location<>'')
			`,
		},
		{
			name: "nullable column",
			tamper: `
				ALTER TABLE workspace_mutation_operations
				ALTER COLUMN project_location DROP NOT NULL
			`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, pool :=
				newWorkspaceMutationProjectLocationRuntimeAuthorityRepository(t)
			if _, err := pool.Exec(t.Context(), test.tamper); err != nil {
				t.Fatal(err)
			}
			assertWorkspaceMutationProjectLocationRuntimeAuthorityError(
				t, repository.ValidateRuntimeAuthority(t.Context()),
			)
		})
	}
}

func TestPostgresStartupAcceptsTerminalWorkspaceMutationAfterProjectLocationChanges(
	t *testing.T,
) {
	fixture := newWorkspaceMutationDatabaseFixture(t, "terminal-location-history")
	var calls workspaceMutationCallbackCounts
	result, err := fixture.repository.ExecuteWorkspaceMutation(
		fixture.ctx, fixture.authority, fixture.command,
		workspaceMutationFixtureCallbacks(fixture, true, &calls),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != workspaceMutationVerified {
		t.Fatalf("terminal workspace mutation status=%q", result.Status)
	}
	if err := fixture.repository.CompleteStep(fixture.ctx, CompleteStepCommand{
		OperationID: testLifecycleOperationID(
			t, "terminal-project-location-history", fixture.command.StepID,
		),
		Authority: fixture.authority, StepID: fixture.command.StepID,
		Output: "verified terminal workspace mutation",
	}); err != nil {
		t.Fatal(err)
	}
	changedLocation := t.TempDir()
	if changedLocation == fixture.command.ProjectLocation {
		t.Fatal("changed terminal project location is not distinct")
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `
		UPDATE projects SET location=$2 WHERE id=$1
	`, fixture.command.ProjectID, changedLocation); err != nil {
		t.Fatal(err)
	}
	if err := fixture.repository.ValidateRuntimeAuthority(fixture.ctx); err != nil {
		t.Fatalf("validate terminal workspace project-location history: %v", err)
	}
}

func TestPostgresStartupRejectsStaleWorkspaceMutationProjectLocationAuthority(
	t *testing.T,
) {
	repository, pool := newWorkspaceMutationProjectLocationRuntimeAuthorityRepository(t)
	fixture := newWorkspaceMutationProjectLocationFixture(t, repository, "startup-stale")
	if _, err := repository.prepareWorkspaceMutation(
		t.Context(), fixture.claim.Authority, fixture.command, fixture.identity,
	); err != nil {
		t.Fatal(err)
	}
	changedLocation := t.TempDir()
	if changedLocation == fixture.projectLocation || changedLocation == fixture.runtimeRoot {
		t.Fatal("changed project location is not distinct")
	}
	if _, err := pool.Exec(t.Context(), `
		ALTER TABLE projects DISABLE TRIGGER projects_active_work_location_guard
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE projects SET location=$2 WHERE id=$1
	`, fixture.command.ProjectID, changedLocation); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		ALTER TABLE projects ENABLE TRIGGER projects_active_work_location_guard
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.ValidateRuntimeAuthority(t.Context())
	if err == nil || !strings.Contains(err.Error(), "stale nonterminal workspace mutation") ||
		!strings.Contains(err.Error(), "current project-location authority") ||
		!strings.Contains(err.Error(), fixture.identity.ID) {
		t.Fatalf("stale workspace project-location startup error=%v", err)
	}
}

func newWorkspaceMutationProjectLocationRuntimeAuthorityRepository(
	t *testing.T,
) (*Repository, *pgxpool.Pool) {
	t.Helper()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "179"),
	); err != nil {
		t.Fatal(err)
	}
	return repository, pool
}

func assertWorkspaceMutationProjectLocationRuntimeAuthorityError(
	t *testing.T,
	err error,
) {
	t.Helper()
	if err == nil || !strings.Contains(
		err.Error(), workspaceMutationProjectLocationDatabaseAuthorityError,
	) {
		t.Fatalf("workspace project-location startup error=%v", err)
	}
}
