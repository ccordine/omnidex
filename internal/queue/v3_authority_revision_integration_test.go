package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestV3AuthorityRevisionCreatesIsolatedSuccessorRun(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL V3 authority revision test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("PostgreSQL unavailable: %v", err)
	}
	repo := New(pool)
	if err := repo.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}

	location := fmt.Sprintf("/tmp/omnidex-authority-%d", time.Now().UnixNano())
	jobIDs := make([]int64, 0, 3)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if len(jobIDs) > 0 {
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM omni_runs WHERE id IN (SELECT NULLIF(metadata->>'telemetry_run_id', '')::uuid FROM jobs WHERE id = ANY($1))`, jobIDs)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM jobs WHERE id = ANY($1)`, jobIDs)
		}
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM projects WHERE location = $1`, location)
	})

	metadata := []byte(fmt.Sprintf(`{"runtime":"v3","client_cwd":%q}`, location))
	original, err := repo.EnqueueJob(ctx, "Repair agent routing", model.PipelineAssistant, metadata)
	if err != nil {
		t.Fatal(err)
	}
	jobIDs = append(jobIDs, original.ID)
	originalSteps := firstRevisionSteps(t, repo, ctx, original.ID)
	if len(originalSteps) == 0 {
		t.Fatal("original V3 job has no steps")
	}
	if err := repo.WriteArtifact(ctx, artifacts.Envelope{
		JobID: original.ID, StepID: originalSteps[0].ID, Kind: artifacts.KindIntent, Version: "test", Payload: json.RawMessage(`{"old":true}`),
	}); err != nil {
		t.Fatal(err)
	}

	firstRevision, err := repo.InterruptJob(ctx, original.ID, "Keep the server authoritative")
	if err != nil {
		t.Fatal(err)
	}
	jobIDs = append(jobIDs, firstRevision.ID)
	if firstRevision.ID == original.ID || firstRevision.Status != model.JobStatusPending {
		t.Fatalf("first revision=%+v", firstRevision)
	}
	originalDetails, err := repo.GetJobDetails(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if originalDetails.Job.Status != model.JobStatusCanceled || !strings.Contains(originalDetails.Job.Error, fmt.Sprintf("job %d", firstRevision.ID)) {
		t.Fatalf("superseded original=%+v", originalDetails.Job)
	}
	if _, found, err := repo.LatestArtifact(ctx, firstRevision.ID, artifacts.KindIntent); err != nil || found {
		t.Fatalf("stale artifact crossed revision boundary: found=%t err=%v", found, err)
	}
	revisionSteps := firstRevisionSteps(t, repo, ctx, firstRevision.ID)
	if len(revisionSteps) == 0 || revisionSteps[0].Action != "v3_intent_parse" {
		t.Fatalf("successor did not restart at intent parsing")
	}

	secondRevision, err := repo.ReplanJob(ctx, firstRevision.ID, "Require independent command evidence")
	if err != nil {
		t.Fatal(err)
	}
	jobIDs = append(jobIDs, secondRevision.ID)
	var successorMetadata struct {
		Revision   int64    `json:"v3_authority_revision"`
		RootJobID  int64    `json:"v3_root_job_id"`
		ParentID   int64    `json:"v3_parent_job_id"`
		Directives []string `json:"v3_authority_directives"`
	}
	if err := json.Unmarshal(secondRevision.Metadata, &successorMetadata); err != nil {
		t.Fatal(err)
	}
	if successorMetadata.Revision != 3 || successorMetadata.RootJobID != original.ID || successorMetadata.ParentID != firstRevision.ID {
		t.Fatalf("successor lineage=%+v", successorMetadata)
	}
	wantDirectives := []string{"Keep the server authoritative", "Require independent command evidence"}
	if !reflect.DeepEqual(successorMetadata.Directives, wantDirectives) {
		t.Fatalf("authority directives=%#v want %#v", successorMetadata.Directives, wantDirectives)
	}

	project, err := repo.GetProjectByLocation(ctx, location)
	if err != nil {
		t.Fatal(err)
	}
	cardID := fmt.Sprintf("authority-card-%d", time.Now().UnixNano())
	if _, err := repo.CreateScrumCard(ctx, project.ID, cardID, "Repair routing", "Keep specialists bound to typed objectives", "in_progress", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	scrumMetadata := []byte(fmt.Sprintf(`{
		"source":"omni-scrum",
		"runtime":"v3",
		"project_id":%d,
		"client_cwd":%q,
		"scrum_card_id":%q,
		"scrum_card_title":"Repair routing",
		"agent_config":{"agent_system":"omnidex"}
	}`, project.ID, location, cardID))
	scrumJob, err := repo.EnqueueJob(ctx, "compiled Scrum history is not authority", model.PipelineScrum, scrumMetadata)
	if err != nil {
		t.Fatal(err)
	}
	jobIDs = append(jobIDs, scrumJob.ID)
	if _, err := repo.UpdateScrumCard(ctx, project.ID, cardID, map[string]any{"job_id": fmt.Sprintf("%d", scrumJob.ID), "play_state": "running"}); err != nil {
		t.Fatal(err)
	}
	scrumRevision, err := repo.InterruptJob(ctx, scrumJob.ID, "Fix only the live routing path")
	if err != nil {
		t.Fatal(err)
	}
	jobIDs = append(jobIDs, scrumRevision.ID)
	card, err := repo.GetScrumCard(ctx, project.ID, cardID)
	if err != nil {
		t.Fatal(err)
	}
	if card.JobID != fmt.Sprintf("%d", scrumRevision.ID) {
		t.Fatalf("Scrum card job binding=%q want %d", card.JobID, scrumRevision.ID)
	}
}

func firstRevisionSteps(t *testing.T, repo *Repository, ctx context.Context, jobID int64) []model.Step {
	t.Helper()
	details, err := repo.GetJobDetails(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	return details.Steps
}
