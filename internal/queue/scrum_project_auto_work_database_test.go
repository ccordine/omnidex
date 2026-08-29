package queue

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresProjectAutoWorkPlayCommitsConfigJobCardAndFlowAtomically(t *testing.T) {
	repository, _, ctx := scrumChannelOperationTestRepository(t)
	project, err := repository.CreateProject(ctx, "Project play", "/tmp/project-auto-play", "")
	if err != nil {
		t.Fatal(err)
	}
	settings := json.RawMessage(`{"scrum_auto_work":{"enabled":false,"source_columns":["ready"]}}`)
	if _, err := repository.UpdateProjectAtRevision(ctx, project.ID, project.UpdatedAt, model.ProjectPatch{Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateScrumCard(ctx, project.ID, "play-card", "Play", "", "ready", nil, nil); err != nil {
		t.Fatal(err)
	}
	result, err := repository.ApplyProjectAutoWork(ctx, ProjectAutoWorkCommand{
		ProjectID: project.ID, Action: ProjectAutoWorkPlay,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Config.Enabled || result.ActiveCard == nil || result.ActiveCard.PlayState != "running" || result.JobID <= 0 {
		t.Fatalf("result=%+v", result)
	}
	storedProject, err := repository.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	storedConfig, err := DecodeScrumAutoWorkConfig(storedProject.Settings)
	if err != nil || !storedConfig.Enabled {
		t.Fatalf("stored config=%+v error=%v", storedConfig, err)
	}
	var metrics ScrumFlowMetrics
	if err := json.Unmarshal(result.ActiveCard.FlowMetrics, &metrics); err != nil {
		t.Fatal(err)
	}
	if metrics.PlayRuns != 1 || metrics.Column != "in_progress" {
		t.Fatalf("play metrics=%+v", metrics)
	}
}

func TestPostgresProjectAutoWorkPauseRejectsMultipleRunningCardsAndRollsBackConfig(t *testing.T) {
	repository, pool, ctx := scrumChannelOperationTestRepository(t)
	project, err := repository.CreateProject(ctx, "Corrupt pause", "/tmp/corrupt-project-pause", "")
	if err != nil {
		t.Fatal(err)
	}
	settings := json.RawMessage(`{"scrum_auto_work":{"enabled":true,"source_columns":["ready"]}}`)
	if _, err := repository.UpdateProjectAtRevision(ctx, project.ID, project.UpdatedAt, model.ProjectPatch{Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	for index, cardID := range []string{"running-one", "running-two"} {
		card, err := repository.CreateScrumCard(ctx, project.ID, cardID, cardID, "", "in_progress", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		job, err := repository.EnqueueJob(
			ctx, "Corrupt running card "+cardID, model.PipelineCoding,
			[]byte(`{"project_id":`+strconv.FormatInt(project.ID, 10)+`}`),
		)
		if err != nil {
			t.Fatal(err)
		}
		jobID := strconv.FormatInt(job.ID, 10)
		if _, err := pool.Exec(ctx, `
			UPDATE scrum_cards SET play_state='running',job_id=$3,sync_job_id=$3,
			 updated_at=GREATEST(clock_timestamp(),updated_at+interval '1 microsecond')
			WHERE project_id=$1 AND id=$2
		`, project.ID, card.ID, jobID); err != nil {
			t.Fatalf("seed running card %d: %v", index, err)
		}
	}
	if _, err := repository.ApplyProjectAutoWork(ctx, ProjectAutoWorkCommand{
		ProjectID: project.ID, Action: ProjectAutoWorkPause,
	}); err == nil {
		t.Fatal("multiple running cards were silently repaired")
	}
	stored, err := repository.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	config, err := DecodeScrumAutoWorkConfig(stored.Settings)
	if err != nil || !config.Enabled {
		t.Fatalf("failed pause changed auto-work config=%+v error=%v", config, err)
	}
	var running int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM scrum_cards WHERE project_id=$1 AND play_state='running'
	`, project.ID).Scan(&running); err != nil {
		t.Fatal(err)
	}
	if running != 2 {
		t.Fatalf("failed pause changed corrupt state running=%d", running)
	}
}

func TestPostgresProjectAutoWorkQueuedStartAdvancesCardHeldMetrics(t *testing.T) {
	repository, pool, ctx := scrumChannelOperationTestRepository(t)
	project, err := repository.CreateProject(ctx, "Queued start", "/tmp/queued-project-start", "")
	if err != nil {
		t.Fatal(err)
	}
	settings := json.RawMessage(`{"scrum_auto_work":{"enabled":false,"source_columns":["ready"]}}`)
	if _, err := repository.UpdateProjectAtRevision(ctx, project.ID, project.UpdatedAt, model.ProjectPatch{Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(ctx, project.ID, "queued-start", "Queued start", "", "ready", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE scrum_cards SET column_name='assigned',play_state='queued',queue_order=7,
		 updated_at=GREATEST(clock_timestamp(),updated_at+interval '1 microsecond')
		WHERE project_id=$1 AND id=$2
	`, project.ID, card.ID); err != nil {
		t.Fatal(err)
	}
	result, err := repository.ApplyProjectAutoWork(ctx, ProjectAutoWorkCommand{
		ProjectID: project.ID, Action: ProjectAutoWorkPlay,
	})
	if err != nil {
		t.Fatal(err)
	}
	var metrics ScrumFlowMetrics
	if result.ActiveCard == nil || result.ActiveCard.PlayState != "running" {
		t.Fatalf("result=%+v", result)
	}
	if err := json.Unmarshal(result.ActiveCard.FlowMetrics, &metrics); err != nil {
		t.Fatal(err)
	}
	if metrics.PlayRuns != 1 || metrics.Column != "in_progress" {
		t.Fatalf("queued start metrics=%+v", metrics)
	}
}

func TestPostgresProjectAutoWorkPlayFailureRollsBackEnabledConfig(t *testing.T) {
	repository, _, ctx := scrumChannelOperationTestRepository(t)
	project, err := repository.CreateProject(ctx, "Project rollback", "/tmp/project-auto-rollback", "")
	if err != nil {
		t.Fatal(err)
	}
	settings := json.RawMessage(`{"scrum_auto_work":{"enabled":false,"source_columns":["ready"]}}`)
	if _, err := repository.UpdateProjectAtRevision(ctx, project.ID, project.UpdatedAt, model.ProjectPatch{Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateScrumCard(ctx, project.ID, "bad-card", "Bad", "", "ready", json.RawMessage(`[{"id":"","text":"bad","done":false}]`), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ApplyProjectAutoWork(ctx, ProjectAutoWorkCommand{
		ProjectID: project.ID, Action: ProjectAutoWorkPlay,
	}); err == nil {
		t.Fatal("invalid selected card unexpectedly started")
	}
	storedProject, err := repository.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	storedConfig, err := DecodeScrumAutoWorkConfig(storedProject.Settings)
	if err != nil || storedConfig.Enabled {
		t.Fatalf("failed play committed config=%+v error=%v", storedConfig, err)
	}
	var jobs int
	if err := repository.pool.QueryRow(ctx, `SELECT COUNT(*) FROM jobs WHERE project_id=$1`, project.ID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Fatalf("failed play committed jobs=%d", jobs)
	}
}

func TestPostgresProjectAutoWorkPauseCommitsConfigAndEveryActiveCardAtomically(t *testing.T) {
	repository, _, ctx := scrumChannelOperationTestRepository(t)
	project, err := repository.CreateProject(ctx, "Project pause", "/tmp/project-auto-pause", "")
	if err != nil {
		t.Fatal(err)
	}
	settings := json.RawMessage(`{"scrum_auto_work":{"enabled":true,"source_columns":["ready"]}}`)
	if _, err := repository.UpdateProjectAtRevision(ctx, project.ID, project.UpdatedAt, model.ProjectPatch{Settings: &settings}); err != nil {
		t.Fatal(err)
	}
	running, err := repository.CreateScrumCard(ctx, project.ID, "running-card", "Run", "", "ready", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	run, err := repository.ApplyScrumCardPlay(ctx, ScrumCardPlayCommand{
		ProjectID: project.ID, CardID: running.ID, ExpectedUpdatedAt: running.UpdatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	queued, err := repository.CreateScrumCard(ctx, project.ID, "queued-card", "Queue", "", "ready", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ApplyScrumCardPlay(ctx, ScrumCardPlayCommand{
		ProjectID: project.ID, CardID: queued.ID, ExpectedUpdatedAt: queued.UpdatedAt,
	}); err != nil {
		t.Fatal(err)
	}
	result, err := repository.ApplyProjectAutoWork(ctx, ProjectAutoWorkCommand{
		ProjectID: project.ID, Action: ProjectAutoWorkPause,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Config.Enabled || result.PausedCards != 2 {
		t.Fatalf("result=%+v", result)
	}
	for _, cardID := range []string{running.ID, queued.ID} {
		card, err := repository.GetScrumCard(ctx, project.ID, cardID)
		if err != nil || card.PlayState != "paused" || card.JobID != "" {
			t.Fatalf("card=%+v error=%v", card, err)
		}
	}
	job, err := repository.CurrentJobDetails(ctx, run.Job.ID)
	if err != nil || job.Job.Status != model.JobStatusCanceled {
		t.Fatalf("job=%+v error=%v", job.Job, err)
	}
}
