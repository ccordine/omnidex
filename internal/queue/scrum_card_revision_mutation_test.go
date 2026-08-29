package queue

import (
	"errors"
	"testing"
	"time"
)

func TestRevisionBoundScrumMutationsRequireRepositoryAndObservedRevision(t *testing.T) {
	t.Parallel()
	repository := (*Repository)(nil)
	title := "new"
	if _, err := repository.UpdateScrumCardAtRevision(
		t.Context(), 1, "card", time.Time{}, ScrumCardRevisionPatch{Title: &title},
	); err == nil || err.Error() != "PostgreSQL, context, project, and card are required for revision-bound Scrum edit" {
		t.Fatalf("nil repository edit error=%v", err)
	}
	if err := repository.DeleteScrumCardAtRevision(t.Context(), 1, "card", time.Time{}); err == nil ||
		err.Error() != "PostgreSQL, context, project, and card are required for revision-bound Scrum deletion" {
		t.Fatalf("nil repository delete error=%v", err)
	}
	repository = &Repository{}
	if _, err := repository.UpdateScrumCardAtRevision(
		t.Context(), 1, "card", time.Time{}, ScrumCardRevisionPatch{Title: &title},
	); err == nil || err.Error() != "PostgreSQL, context, project, and card are required for revision-bound Scrum edit" {
		t.Fatalf("uninitialized repository edit error=%v", err)
	}
}

func TestPostgresRevisionBoundScrumEditAndDeleteRejectStaleState(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	ctx := t.Context()
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "089")); err != nil {
		t.Fatal(err)
	}
	var projectID int64
	if err := repository.pool.QueryRow(ctx, `
		INSERT INTO projects(location,name) VALUES('/tmp/scrum-revision','Scrum revision') RETURNING id
	`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(ctx, projectID, "revision-card", "Original", "state", "backlog", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repository.UpdateScrumCardAtRevision(
		ctx, projectID, card.ID, card.UpdatedAt, ScrumCardRevisionPatch{Title: ptrScrumRevisionText("Edited")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Edited" || !updated.UpdatedAt.After(card.UpdatedAt) {
		t.Fatalf("revision-bound edit=%+v baseline=%+v", updated, card)
	}
	if _, err := repository.UpdateScrumCardAtRevision(
		ctx, projectID, card.ID, card.UpdatedAt, ScrumCardRevisionPatch{Title: ptrScrumRevisionText("Stale")},
	); !errors.Is(err, ErrScrumCardVersionConflict) {
		t.Fatalf("stale edit error=%v", err)
	}
	if err := repository.DeleteScrumCardAtRevision(ctx, projectID, card.ID, card.UpdatedAt); !errors.Is(err, ErrScrumCardVersionConflict) {
		t.Fatalf("stale delete error=%v", err)
	}
	afterStale, err := repository.GetScrumCard(ctx, projectID, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterStale.Title != "Edited" || !afterStale.UpdatedAt.Equal(updated.UpdatedAt) {
		t.Fatalf("stale mutation changed card: %+v", afterStale)
	}
	if err := repository.DeleteScrumCardAtRevision(ctx, projectID, card.ID, updated.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetScrumCard(ctx, projectID, card.ID); err == nil {
		t.Fatal("revision-bound delete left card present")
	}
	if _, err := repository.CreateScrumCard(ctx, projectID, card.ID, "Reused", "", "backlog", nil, nil); err != nil {
		t.Fatalf("reuse identity without retained evidence: %v", err)
	}
}

func TestPostgresRevisionBoundScrumDeleteRejectsActiveCardAtomically(t *testing.T) {
	repository, _, ctx := scrumChannelOperationTestRepository(t)
	project, err := repository.CreateProject(ctx, "Active delete", "/tmp/active-card-delete", "")
	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(ctx, project.ID, "active-delete-card", "Active", "", "ready", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	started, err := repository.ApplyScrumCardPlay(ctx, ScrumCardPlayCommand{
		ProjectID: project.ID, CardID: card.ID, ExpectedUpdatedAt: card.UpdatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.DeleteScrumCardAtRevision(ctx, project.ID, card.ID, started.Card.UpdatedAt); !errors.Is(err, ErrScrumCardActiveDelete) {
		t.Fatalf("active delete error=%v", err)
	}
	after, err := repository.GetScrumCard(ctx, project.ID, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.PlayState != "running" || after.SyncJobID == "" || !after.UpdatedAt.Equal(started.Card.UpdatedAt) {
		t.Fatalf("rejected deletion changed active card: %+v", after)
	}
}

func ptrScrumRevisionText(value string) *string { return &value }
