package queue

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresProjectUpdateAndDeleteRequireExactObservedRevision(t *testing.T) {
	repository, _, ctx := scrumChannelOperationTestRepository(t)
	project, err := repository.CreateProject(ctx, "Revision project", "/tmp/project-revision", "exact")
	if err != nil {
		t.Fatal(err)
	}
	changed := "exact changed"
	updated, err := repository.UpdateProjectAtRevision(ctx, project.ID, project.UpdatedAt, model.ProjectPatch{Description: &changed})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Description != changed || !updated.UpdatedAt.After(project.UpdatedAt) {
		t.Fatalf("updated project=%+v", updated)
	}
	if _, err := repository.UpdateProjectAtRevision(ctx, project.ID, project.UpdatedAt, model.ProjectPatch{Description: &changed}); !errors.Is(err, ErrProjectVersionConflict) {
		t.Fatalf("stale project update error=%v", err)
	}
	if err := repository.DeleteProjectAtRevision(ctx, project.ID, project.UpdatedAt); !errors.Is(err, ErrProjectVersionConflict) {
		t.Fatalf("stale project delete error=%v", err)
	}
	if err := repository.DeleteProjectAtRevision(ctx, project.ID, updated.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetProject(ctx, project.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("deleted project lookup error=%v", err)
	}
}

func TestPostgresProjectDeleteRejectsActiveCardAndNonterminalJobAtomically(t *testing.T) {
	repository, pool, ctx := scrumChannelOperationTestRepository(t)
	activeProject, err := repository.CreateProject(ctx, "Active card project", "/tmp/project-delete-active-card", "")
	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(ctx, activeProject.ID, "project-active-card", "Active", "", "ready", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE scrum_cards SET play_state='queued',updated_at=GREATEST(clock_timestamp(),updated_at+interval '1 microsecond')
		WHERE project_id=$1 AND id=$2
	`, activeProject.ID, card.ID); err != nil {
		t.Fatal(err)
	}
	activeProject, err = repository.GetProject(ctx, activeProject.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.DeleteProjectAtRevision(ctx, activeProject.ID, activeProject.UpdatedAt); !errors.Is(err, ErrProjectActiveWork) {
		t.Fatalf("active-card project delete error=%v", err)
	}
	if _, err := repository.GetProject(ctx, activeProject.ID); err != nil {
		t.Fatalf("rejected active-card project delete mutated project: %v", err)
	}

	jobProject, err := repository.CreateProject(ctx, "Active job project", "/tmp/project-delete-active-job", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.EnqueueJob(ctx, "nonterminal deletion fence", model.PipelineCoding, []byte(`{"project_id":`+fmt.Sprint(jobProject.ID)+`}`)); err != nil {
		t.Fatal(err)
	}
	jobProject, err = repository.GetProject(ctx, jobProject.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.DeleteProjectAtRevision(ctx, jobProject.ID, jobProject.UpdatedAt); !errors.Is(err, ErrProjectActiveWork) {
		t.Fatalf("nonterminal-job project delete error=%v", err)
	}
	if _, err := repository.GetProject(ctx, jobProject.ID); err != nil {
		t.Fatalf("rejected nonterminal-job project delete mutated project: %v", err)
	}
}

func TestPostgresProjectDeleteCascadesCurrentScrumStateAndRetainsMinimalReceipt(t *testing.T) {
	repository, pool, ctx := scrumChannelOperationTestRepository(t)
	project, card := newScrumChannelOperationCard(t, repository, "project-delete")
	request := ScrumChannelOperationRequest{
		OperationID: testLifecycleOperationID(t, "project-delete-receipt", project.ID),
		ProjectID:   project.ID, CardID: card.ID, Message: "Start before project deletion.",
	}
	started, err := repository.ExecuteScrumChannelOperation(ctx, ScrumChannelOperationCommand{
		Request: request, ExpectedCardUpdatedAt: card.UpdatedAt,
		Effect:       ScrumChannelEffect{Kind: ScrumChannelStartJob, Instruction: request.Message},
		ResultAction: "started",
	}, func(current DBScrumCard, job model.Job) (ScrumChannelCardUpdate, error) {
		return scrumChannelTestUpdate(t, current, request, job), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.PauseScrumCardPlayAtRevision(ctx, project.ID, card.ID, started.Card.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	project, err = repository.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.DeleteProjectAtRevision(ctx, project.ID, project.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	var cards, messages, operations, registry, detachedJobs int
	if err := pool.QueryRow(ctx, `
		SELECT
		 (SELECT COUNT(*) FROM scrum_cards WHERE project_id=$1 AND id=$2),
		 (SELECT COUNT(*) FROM scrum_card_messages WHERE project_id=$1 AND card_id=$2),
		 (SELECT COUNT(*) FROM scrum_channel_operations WHERE operation_id=$3),
		 (SELECT COUNT(*) FROM lifecycle_operation_registry WHERE operation_id=$3),
		 (SELECT COUNT(*) FROM jobs WHERE id=$4 AND project_id IS NULL)
	`, project.ID, card.ID, request.OperationID, started.Job.ID).Scan(
		&cards, &messages, &operations, &registry, &detachedJobs,
	); err != nil {
		t.Fatal(err)
	}
	if cards != 0 || messages != 0 || operations != 1 || registry != 1 || detachedJobs != 1 {
		t.Fatalf("project deletion cards/messages/receipt/registry/detached_job=%d/%d/%d/%d/%d",
			cards, messages, operations, registry, detachedJobs)
	}
}
