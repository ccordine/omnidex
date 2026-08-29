package queue

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/scrum"
	"github.com/jackc/pgx/v5/pgxpool"
)

func scrumChannelOperationTestRepository(t *testing.T) (*Repository, *pgxpool.Pool, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	return repository, pool, ctx
}

func newScrumChannelOperationCard(t *testing.T, repository *Repository, label string) (model.Project, DBScrumCard) {
	t.Helper()
	project, err := repository.CreateProject(
		t.Context(), fmt.Sprintf("scrum-channel-%s-%d", label, time.Now().UnixNano()), t.TempDir(), "")

	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(
		t.Context(), project.ID, "", "Channel operation", "", "assigned", nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return project, card
}

func enqueueScrumChannelJobForTest(
	t *testing.T,
	repository *Repository,
	card DBScrumCard,
	instruction string,
) model.Job {
	t.Helper()
	job, err := repository.EnqueueScrumJob(t.Context(), instruction, scrum.JobMetadata{
		Source: scrum.JobMetadataSource, ProjectID: card.ProjectID, CardID: card.ID,
		CardTitle: card.Title, CardDescription: card.Description,
		ReturnColumn: card.Column, ChannelOrigin: false, ModelConfig: modelconfig.Config{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func scrumChannelTestUpdate(
	t *testing.T,
	current DBScrumCard,
	request ScrumChannelOperationRequest,
	job model.Job,
) ScrumChannelCardUpdate {
	t.Helper()
	return ScrumChannelCardUpdate{
		Messages: []ScrumCardMessageAppend{{
			ID: "message-" + string(request.OperationID), Role: "user", Content: request.Message,
			OperationID: string(request.OperationID),
		}}, Column: "in_progress", JobID: fmt.Sprintf("%d", job.ID),
		PlayState: "running", QueueOrder: 0, SyncJobID: fmt.Sprintf("%d", job.ID),
	}
}

func assertScrumChannelOperationCounts(
	t *testing.T,
	pool *pgxpool.Pool,
	projectID int64,
	cardID string,
	wantOperations, wantMessages, wantJobs int,
) {
	t.Helper()
	var operations, messages, jobs int
	if err := pool.QueryRow(t.Context(), `
		SELECT
			(SELECT COUNT(*) FROM scrum_channel_operations WHERE project_id=$1 AND card_id=$2),
			(SELECT channel_message_count FROM scrum_cards WHERE project_id=$1 AND id=$2),
			(SELECT COUNT(*) FROM jobs WHERE project_id=$1)
	`, projectID, cardID).Scan(&operations, &messages, &jobs); err != nil {
		t.Fatal(err)
	}
	if operations != wantOperations || messages != wantMessages || jobs != wantJobs {
		t.Fatalf("counts operations=%d messages=%d jobs=%d", operations, messages, jobs)
	}
}
