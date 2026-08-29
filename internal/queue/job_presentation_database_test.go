package queue

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresCurrentJobPresentationReturnsOnlyLatestCurrentGenerationProgress(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	job, err := repository.EnqueueJob(ctx, "bounded presentation", model.PipelineCoding, nil)
	if err != nil {
		t.Fatal(err)
	}
	var stepID int64
	if err := pool.QueryRow(ctx, `
		SELECT id FROM job_steps WHERE job_id=$1 AND generation=$2 ORDER BY id LIMIT 1
	`, job.ID, job.CurrentGeneration).Scan(&stepID); err != nil {
		t.Fatal(err)
	}

	inserted := make([]int64, 0, maxCurrentJobProgressItems+6)
	for index := 0; index < maxCurrentJobProgressItems+6; index++ {
		var contextID int64
		value := fmt.Sprintf(
			"time=%s event=coding_file_written path=file_%02d.go bytes=12 operation=create result=accepted",
			time.Now().UTC().Format(time.RFC3339), index,
		)
		if err := pool.QueryRow(ctx, `
			INSERT INTO step_contexts (step_id,key,value) VALUES ($1,'event',$2) RETURNING id
		`, stepID, value).Scan(&contextID); err != nil {
			t.Fatal(err)
		}
		inserted = append(inserted, contextID)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO step_contexts (step_id,key,value) VALUES ($1,'llm_prompt',$2)
	`, stepID, strings.Repeat("private model envelope", 1000)); err != nil {
		t.Fatal(err)
	}

	presentation, err := repository.CurrentJobPresentation(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if presentation.Job.ID != job.ID || presentation.Job.CurrentGeneration != job.CurrentGeneration {
		t.Fatalf("presentation job=%+v, want job=%d generation=%d", presentation.Job, job.ID, job.CurrentGeneration)
	}
	if presentation.Progress.JobID != job.ID || presentation.Progress.Generation != job.CurrentGeneration {
		t.Fatalf("progress authority=%+v", presentation.Progress)
	}
	if len(presentation.Progress.Items) != maxCurrentJobProgressItems {
		t.Fatalf("progress items=%d want %d", len(presentation.Progress.Items), maxCurrentJobProgressItems)
	}
	wantFirst := inserted[len(inserted)-maxCurrentJobProgressItems]
	wantLast := inserted[len(inserted)-1]
	if presentation.Progress.Items[0].Context.ID != wantFirst ||
		presentation.Progress.LatestContextID != wantLast {
		t.Fatalf("progress window first=%d latest=%d, want %d..%d",
			presentation.Progress.Items[0].Context.ID,
			presentation.Progress.LatestContextID,
			wantFirst, wantLast,
		)
	}
	for _, item := range presentation.Progress.Items {
		if item.Context.Key != "event" || item.Generation != job.CurrentGeneration || item.StepAction == "" {
			t.Fatalf("progress escaped typed current authority: %+v", item)
		}
	}
}

func TestPostgresCurrentJobPresentationRejectsOversizedProgressAuthority(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	job, err := repository.EnqueueJob(ctx, "oversized presentation", model.PipelineCoding, nil)
	if err != nil {
		t.Fatal(err)
	}
	var stepID int64
	if err := pool.QueryRow(ctx, `
		SELECT id FROM job_steps WHERE job_id=$1 AND generation=$2 ORDER BY id LIMIT 1
	`, job.ID, job.CurrentGeneration).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO step_contexts (step_id,key,value) VALUES ($1,'event',$2)
	`, stepID, strings.Repeat("x", maxCurrentJobProgressValueBytes+1)); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CurrentJobPresentation(ctx, job.ID); !errors.Is(err, ErrInvalidJobPresentation) {
		t.Fatalf("CurrentJobPresentation error=%v, want ErrInvalidJobPresentation", err)
	}
}
