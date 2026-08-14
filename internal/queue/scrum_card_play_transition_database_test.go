package queue

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresScrumCardPlayStartsAndPausesAtomicallyAtRevision(t *testing.T) {
	repository, _, ctx := scrumChannelOperationTestRepository(t)
	project, err := repository.CreateProject(ctx, "Play", "/tmp/scrum-play-atomic", "")
	if err != nil {
		t.Fatal(err)
	}
	settings := json.RawMessage(`{"model_config":{"coding_fragment_model":"locked-model"}}`)
	if _, err := repository.UpdateProjectAtRevision(ctx, project.ID, project.UpdatedAt, model.ProjectPatch{Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(ctx, project.ID, "play-card", "Play card", "Exact work", "ready", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := repository.ApplyScrumCardPlay(ctx, ScrumCardPlayCommand{
		ProjectID: project.ID, CardID: card.ID, ExpectedUpdatedAt: card.UpdatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != "started" || result.Card.PlayState != "running" ||
		result.Card.JobID != strconv.FormatInt(result.Job.ID, 10) || result.Card.SyncJobID != result.Card.JobID {
		t.Fatalf("start result=%+v", result)
	}
	metadata := decodeMetadataObject(result.Job.Metadata)
	modelConfig, ok := metadata["model_config"].(map[string]any)
	if !ok || modelConfig["coding_fragment_model"] != "locked-model" {
		t.Fatalf("locked model snapshot=%#v", metadata["model_config"])
	}
	if _, err := repository.ApplyScrumCardPlay(ctx, ScrumCardPlayCommand{
		ProjectID: project.ID, CardID: card.ID, ExpectedUpdatedAt: card.UpdatedAt,
	}); err == nil {
		t.Fatal("stale Play revision was accepted")
	}
	paused, err := repository.PauseScrumCardPlayAtRevision(ctx, project.ID, card.ID, result.Card.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	if paused.PlayState != "paused" || paused.Column != "assigned" || paused.JobID != "" || paused.SyncJobID != "" {
		t.Fatalf("paused card=%+v", paused)
	}
	job, err := repository.CurrentJobDetails(ctx, result.Job.ID)
	if err != nil || job.Job.Status != model.JobStatusCanceled {
		t.Fatalf("canceled job=%+v error=%v", job.Job, err)
	}
}

func TestPostgresScrumCardPlayRejectsAuthoritativeGlobalPauseWithoutRows(t *testing.T) {
	repository, pool, ctx := scrumChannelOperationTestRepository(t)
	project, err := repository.CreateProject(ctx, "Paused play", "/tmp/scrum-play-paused", "")
	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(ctx, project.ID, "paused-card", "Paused card", "", "ready", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SetAIPaused(ctx, true); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ApplyScrumCardPlay(ctx, ScrumCardPlayCommand{
		ProjectID: project.ID, CardID: card.ID, ExpectedUpdatedAt: card.UpdatedAt,
	}); err == nil {
		t.Fatal("globally paused Scrum play was accepted")
	}
	var jobs, messages int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM jobs WHERE project_id=$1),
			(SELECT COUNT(*) FROM scrum_card_messages WHERE project_id=$1 AND card_id=$2)
	`, project.ID, card.ID).Scan(&jobs, &messages); err != nil {
		t.Fatal(err)
	}
	unchanged, err := repository.GetScrumCard(ctx, project.ID, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if jobs != 0 || messages != 0 || unchanged.PlayState != "" || unchanged.JobID != "" {
		t.Fatalf("paused play mutated state jobs=%d messages=%d card=%+v", jobs, messages, unchanged)
	}
}

func TestPostgresScrumCardPlayQueuesAndPivotRollsBackOnInvalidTarget(t *testing.T) {
	repository, _, ctx := scrumChannelOperationTestRepository(t)
	project, err := repository.CreateProject(ctx, "Pivot", "/tmp/scrum-play-pivot", "")
	if err != nil {
		t.Fatal(err)
	}
	running, err := repository.CreateScrumCard(ctx, project.ID, "running-card", "Running", "", "ready", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	run, err := repository.ApplyScrumCardPlay(ctx, ScrumCardPlayCommand{
		ProjectID: project.ID, CardID: running.ID, ExpectedUpdatedAt: running.UpdatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	target, err := repository.CreateScrumCard(ctx, project.ID, "target-card", "Target", "", "ready", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := repository.ApplyScrumCardPlay(ctx, ScrumCardPlayCommand{
		ProjectID: project.ID, CardID: target.ID, ExpectedUpdatedAt: target.UpdatedAt,
	})
	if err != nil || queued.Action != "queued" || queued.QueuePosition != 1 {
		t.Fatalf("queued=%+v error=%v", queued, err)
	}
	if _, err := repository.pool.Exec(ctx, `
		UPDATE scrum_cards SET column_name='blocked' WHERE project_id=$1 AND id=$2
	`, project.ID, target.ID); err != nil {
		t.Fatal(err)
	}
	invalidTarget, err := repository.GetScrumCard(ctx, project.ID, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.ApplyScrumCardPlay(ctx, ScrumCardPlayCommand{
		ProjectID: project.ID, CardID: target.ID, ExpectedUpdatedAt: invalidTarget.UpdatedAt, Pivot: true,
	})
	if err == nil {
		t.Fatal("invalid pivot target was accepted")
	}
	currentRunning, err := repository.GetScrumCard(ctx, project.ID, running.ID)
	if err != nil || currentRunning.PlayState != "running" || currentRunning.JobID != fmt.Sprintf("%d", run.Job.ID) {
		t.Fatalf("failed pivot changed running card=%+v error=%v", currentRunning, err)
	}
	job, err := repository.CurrentJobDetails(ctx, run.Job.ID)
	if err != nil || job.Job.Status == model.JobStatusCanceled {
		t.Fatalf("failed pivot canceled running job=%+v error=%v", job.Job, err)
	}
}
