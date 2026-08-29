package queue

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresProjectLocationGuardRejectsActiveWork(t *testing.T) {
	fixture := newWorkspaceMutationDatabaseFixture(t, "active-project-location")
	changedLocation := t.TempDir()
	if _, err := fixture.pool.Exec(fixture.ctx, `
		UPDATE projects SET location=$2 WHERE id=$1
	`, fixture.command.ProjectID, changedLocation); err == nil ||
		!strings.Contains(err.Error(), "project location cannot change while job") {
		t.Fatalf("direct active-work project-location error=%v", err)
	}
	project, err := fixture.repository.GetProject(fixture.ctx, fixture.command.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.UpdateProjectAtRevision(
		fixture.ctx, project.ID, project.UpdatedAt,
		model.ProjectPatch{Location: &changedLocation},
	); !errors.Is(err, ErrProjectActiveWork) {
		t.Fatalf("revision-bound active-work project-location error=%v", err)
	}
	retained, err := fixture.repository.GetProject(fixture.ctx, fixture.command.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if retained.Location != fixture.command.ProjectLocation {
		t.Fatalf("rejected project-location update retained %q", retained.Location)
	}
}

func TestPostgresWorkspaceMutationPrepareRejectsChangedProjectLocation(t *testing.T) {
	fixture := newWorkspaceMutationDatabaseFixture(t, "project-location-race")
	identity, err := workspaceMutationOperation(fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	changedLocation := t.TempDir()
	if _, err := fixture.pool.Exec(fixture.ctx, `
		ALTER TABLE projects DISABLE TRIGGER projects_active_work_location_guard
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `
		UPDATE projects SET location=$2 WHERE id=$1
	`, fixture.command.ProjectID, changedLocation); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.pool.Exec(fixture.ctx, `
		ALTER TABLE projects ENABLE TRIGGER projects_active_work_location_guard
	`); err != nil {
		t.Fatal(err)
	}

	if _, err := fixture.repository.prepareWorkspaceMutation(
		fixture.ctx, fixture.authority, fixture.command, identity,
	); !errors.Is(err, ErrWorkspaceMutationConflict) {
		t.Fatalf("changed project-location preparation error=%v, want ErrWorkspaceMutationConflict", err)
	}
	requireNoWorkspaceMutationOperation(t, fixture.pool, identity.ID)
}
