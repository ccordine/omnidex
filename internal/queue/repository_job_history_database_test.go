package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type jobHistoryFixture struct {
	stepID     int64
	artifactID int64
	evidenceID int64
	llmID      int64
}

func TestPostgresJobHistoryIsCursorPagedAndStepResolved(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("job-history-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	oldStepID := historyStepID(t, pool, job.ID, 1)
	old := insertJobHistoryFixture(t, repository, pool, job.ID, oldStepID, marker+"-old")
	if _, err := repository.ReplanJob(ctx, testReplanCommand(t, job.ID, "history", "Create a fresh generation for history inspection.")); err != nil {
		t.Fatal(err)
	}
	currentStepID := historyStepID(t, pool, job.ID, 2)
	current := insertJobHistoryFixture(t, repository, pool, job.ID, currentStepID, marker+"-current")

	firstGeneration, err := repository.ReadJobHistoryPage(ctx, job.ID, JobHistoryRequest{
		Stream: JobHistoryGenerations, Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(firstGeneration.Generations) != 1 || firstGeneration.Generations[0].Generation != 1 ||
		firstGeneration.NextCursor == "" {
		t.Fatalf("first generation page=%+v", firstGeneration)
	}
	secondGeneration, err := repository.ReadJobHistoryPage(ctx, job.ID, JobHistoryRequest{
		Stream: JobHistoryGenerations, Limit: 1, Cursor: firstGeneration.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(secondGeneration.Generations) != 1 || secondGeneration.Generations[0].Generation != 2 ||
		secondGeneration.Generations[0].PredecessorGeneration == nil ||
		*secondGeneration.Generations[0].PredecessorGeneration != 1 || secondGeneration.NextCursor != "" {
		t.Fatalf("second generation page=%+v", secondGeneration)
	}

	steps, err := repository.ReadJobHistoryPage(ctx, job.ID, JobHistoryRequest{
		Stream: JobHistorySteps, Limit: MaxJobHistoryPageSize,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertHistoricalStep(t, steps.Steps, old.stepID, 1, int64Pointer(2))
	assertHistoricalStep(t, steps.Steps, current.stepID, 2, nil)

	assertRecordHistoryPages(t, repository, job.ID, old, current)

	if _, err := repository.ReadJobHistoryPage(ctx, job.ID+9_000_000_000, JobHistoryRequest{
		Stream: JobHistorySteps, Limit: 1,
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing job history error=%v", err)
	}
}

func TestPostgresArtifactHistoryUsesJobIDKeysetIndex(t *testing.T) {
	_, pool, ctx := replanTestRepository(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SET LOCAL enable_seqscan=off`); err != nil {
		t.Fatal(err)
	}
	rows, err := tx.Query(ctx, `
		EXPLAIN (COSTS OFF)
		SELECT artifact.id, artifact.job_id, artifact.step_id
		FROM artifacts AS artifact
		JOIN job_steps AS steps
		  ON steps.job_id=artifact.job_id AND steps.id=artifact.step_id
		WHERE artifact.job_id=$1 AND artifact.id>$2
		ORDER BY artifact.id ASC
		LIMIT $3
	`, int64(1), int64(0), 10)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), "idx_artifacts_job_id_id") {
		t.Fatalf("artifact history plan omitted keyset index:\n%s", plan.String())
	}
}

func assertRecordHistoryPages(
	t *testing.T,
	repository *Repository,
	jobID int64,
	old, current jobHistoryFixture,
) {
	t.Helper()
	tests := []struct {
		stream JobHistoryStream
		oldID  int64
		newID  int64
		ref    func(JobHistoryPage) HistoricalStepReference
		id     func(JobHistoryPage) int64
	}{
		{JobHistoryArtifacts, old.artifactID, current.artifactID,
			func(page JobHistoryPage) HistoricalStepReference { return page.Artifacts[0].Step },
			func(page JobHistoryPage) int64 {
				id, _ := strconv.ParseInt(page.Artifacts[0].Artifact.ID, 10, 64)
				return id
			}},
		{JobHistoryEvidence, old.evidenceID, current.evidenceID,
			func(page JobHistoryPage) HistoricalStepReference { return page.Evidence[0].Step },
			func(page JobHistoryPage) int64 { return page.Evidence[0].Evidence.ID }},
		{JobHistoryLLMCalls, old.llmID, current.llmID,
			func(page JobHistoryPage) HistoricalStepReference { return page.LLMCalls[0].Step },
			func(page JobHistoryPage) int64 { return page.LLMCalls[0].Call.ID }},
	}
	for _, test := range tests {
		t.Run(string(test.stream), func(t *testing.T) {
			first, err := repository.ReadJobHistoryPage(t.Context(), jobID, JobHistoryRequest{Stream: test.stream, Limit: 1})
			if err != nil {
				t.Fatal(err)
			}
			if first.NextCursor == "" {
				t.Fatalf("first page has no cursor: %+v", first)
			}
			if test.id(first) != test.oldID {
				t.Fatalf("first record ID=%d want %d", test.id(first), test.oldID)
			}
			assertHistoryReference(t, test.ref(first), old.stepID, 1, int64Pointer(2))
			second, err := repository.ReadJobHistoryPage(t.Context(), jobID, JobHistoryRequest{
				Stream: test.stream, Limit: 1, Cursor: first.NextCursor,
			})
			if err != nil {
				t.Fatal(err)
			}
			if second.NextCursor != "" {
				t.Fatalf("last page retained cursor: %+v", second)
			}
			if test.id(second) != test.newID {
				t.Fatalf("second record ID=%d want %d", test.id(second), test.newID)
			}
			assertHistoryReference(t, test.ref(second), current.stepID, 2, nil)
		})
	}
}

func insertJobHistoryFixture(
	t *testing.T,
	repository *Repository,
	pool *pgxpool.Pool,
	jobID, stepID int64,
	marker string,
) jobHistoryFixture {
	t.Helper()
	ctx := t.Context()
	fixture := jobHistoryFixture{stepID: stepID}
	if err := pool.QueryRow(ctx, `
		INSERT INTO artifacts (job_id, step_id, kind, version, payload_json)
		VALUES ($1, $2, 'analysis', 'v1', $3::jsonb)
		RETURNING id
	`, jobID, stepID, fmt.Sprintf(`{"marker":%q}`, marker)).Scan(&fixture.artifactID); err != nil {
		t.Fatal(err)
	}
	record := evidence.Record{
		JobID: jobID, StepID: stepID, Kind: evidence.KindModelJudgment,
		SourceType: "history-fixture", SourceRef: marker, Summary: marker,
	}
	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO evidence (job_id, step_id, kind, source_type, source_ref, payload_json)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)
		RETURNING id
	`, jobID, stepID, record.Kind, record.SourceType, record.SourceRef, payload).Scan(&fixture.evidenceID); err != nil {
		t.Fatal(err)
	}
	var generation int64
	if err := pool.QueryRow(ctx, `
		SELECT generation FROM job_steps WHERE job_id=$1 AND id=$2
	`, jobID, stepID).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	authority := activateStepAttemptForTest(
		t, ctx, pool, jobID, generation, stepID,
		testStepAttemptWorker("job-history", stepID),
	)
	evidenceFixture := prepareSuccessfulStationEvidenceFixture(
		t, repository, authority, newStationEvidenceJobForTest(t, marker),
		`{"schema":"omnidex.conversation-response.v1","text":"history fixture"}`,
	)
	llmCall := persistPreparedStationEvidenceFixture(t, repository, evidenceFixture, "")
	fixture.llmID = llmCall.ID
	return fixture
}

func historyStepID(t *testing.T, pool *pgxpool.Pool, jobID, generation int64) int64 {
	t.Helper()
	var stepID int64
	if err := pool.QueryRow(t.Context(), `
		SELECT id FROM job_steps
		WHERE job_id=$1 AND generation=$2 AND action='v3_coding'
		ORDER BY id ASC LIMIT 1
	`, jobID, generation).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	return stepID
}

func assertHistoricalStep(
	t *testing.T,
	steps []HistoricalStep,
	stepID, generation int64,
	supersededAt *int64,
) {
	t.Helper()
	for _, step := range steps {
		if step.StepID == stepID {
			assertHistoryReference(t, step.HistoricalStepReference, stepID, generation, supersededAt)
			return
		}
	}
	t.Fatalf("historical step %d was not returned", stepID)
}

func assertHistoryReference(
	t *testing.T,
	reference HistoricalStepReference,
	stepID, generation int64,
	supersededAt *int64,
) {
	t.Helper()
	if reference.StepID != stepID || reference.Generation != generation {
		t.Fatalf("step reference=%+v want step=%d generation=%d", reference, stepID, generation)
	}
	if (reference.SupersededAtGeneration == nil) != (supersededAt == nil) ||
		(reference.SupersededAtGeneration != nil && *reference.SupersededAtGeneration != *supersededAt) {
		t.Fatalf("step reference supersession=%v want %v", reference.SupersededAtGeneration, supersededAt)
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}
