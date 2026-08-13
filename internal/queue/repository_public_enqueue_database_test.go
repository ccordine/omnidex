package queue

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresPublicEnqueueRejectsFreeFormTransportsBeforePersistence(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	var before int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM jobs`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, pipeline := range []string{model.PipelineAssistant, model.PipelineChat, model.PipelineStory} {
		if _, err := repository.EnqueueJob(ctx, "must not persist", pipeline, []byte(`{}`)); !errors.Is(err, ErrChannelTransportRequired) {
			t.Fatalf("pipeline %q error=%v", pipeline, err)
		}
	}
	var after int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM jobs`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("rejected public enqueues persisted jobs: before=%d after=%d", before, after)
	}
}

func TestPostgresHistoricalFreeFormJobsRemainReadable(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	var historicalID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO jobs(instruction,pipeline,status,metadata,current_generation)
		VALUES ('historical assistant result','assistant','completed','{}',1)
		RETURNING id
	`).Scan(&historicalID); err != nil {
		t.Fatal(err)
	}
	jobs, err := repository.ListJobs(ctx, "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range jobs {
		if job.ID == historicalID && job.Pipeline == model.PipelineAssistant {
			return
		}
	}
	t.Fatalf("historical assistant job %d was hidden: %+v", historicalID, jobs)
}
