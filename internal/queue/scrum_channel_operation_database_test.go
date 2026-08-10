package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresScrumChannelOperationStartsOnceAndReplaysExactly(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	project, card := newScrumChannelOperationCard(t, repository, "start")
	request := ScrumChannelOperationRequest{
		OperationID: testLifecycleOperationID(t, "scrum-channel-start", project.ID),
		ProjectID:   project.ID,
		CardID:      card.ID,
		Message:     "Start this card once.",
	}
	command := ScrumChannelOperationCommand{
		Request:               request,
		ExpectedCardUpdatedAt: card.UpdatedAt,
		Effect: ScrumChannelEffect{
			Kind:        ScrumChannelStartJob,
			Instruction: request.Message,
			Pipeline:    model.PipelineAssistant,
			Metadata:    json.RawMessage(fmt.Sprintf(`{"project_id":%d}`, project.ID)),
		},
		ResultAction: "started",
		ResultAgent:  "omnidex",
	}
	builderCalls := 0
	builder := func(current DBScrumCard, job model.Job) (ScrumChannelCardUpdate, error) {
		builderCalls++
		return scrumChannelTestUpdate(t, current, request, job), nil
	}

	first, err := repository.ExecuteScrumChannelOperation(ctx, command, builder)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.ExecuteScrumChannelOperation(ctx, command, builder)
	if err != nil {
		t.Fatalf("exact replay: %v", err)
	}
	if !first.Applied || second.Applied || builderCalls != 1 {
		t.Fatalf("applied first=%t second=%t builder_calls=%d", first.Applied, second.Applied, builderCalls)
	}
	if first.Job.ID != second.Job.ID || first.Card.ID != second.Card.ID ||
		!first.Card.UpdatedAt.Equal(second.Card.UpdatedAt) || string(first.Card.Chat) != string(second.Card.Chat) {
		t.Fatalf("replay mismatch first=%+v second=%+v", first, second)
	}
	assertScrumChannelOperationCounts(t, pool, project.ID, card.ID, 1, 1, 1)
	if _, err := repository.ReplanJob(ctx, ReplanJobCommand{
		OperationID: request.OperationID, JobID: first.Job.ID,
		Feedback: "Attempt to reuse the submitted card identity as a direct replan.",
	}); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatalf("cross-kind lifecycle identity reuse error=%v", err)
	}

	changed := command
	changed.Request.Message = "Changed content under the same identity."
	if _, err := repository.ExecuteScrumChannelOperation(ctx, changed, builder); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatalf("changed replay error=%v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE scrum_channel_operations SET result_action='changed' WHERE operation_id=$1`, request.OperationID); err == nil {
		t.Fatal("immutable Scrum channel operation accepted UPDATE")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM scrum_channel_operations WHERE operation_id=$1`, request.OperationID); err == nil {
		t.Fatal("immutable Scrum channel operation accepted DELETE")
	}
}

func TestPostgresScrumChannelOperationReplansSameJobOnce(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	project, card := newScrumChannelOperationCard(t, repository, "replan")
	job, err := repository.EnqueueJob(
		ctx,
		"Initial Scrum channel job.",
		model.PipelineAssistant,
		[]byte(fmt.Sprintf(`{"project_id":%d}`, project.ID)),
	)
	if err != nil {
		t.Fatal(err)
	}
	card, err = repository.UpdateScrumCard(ctx, project.ID, card.ID, map[string]any{
		"job_id": fmt.Sprintf("%d", job.ID), "play_state": "running", "column": "in_progress",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := ScrumChannelOperationRequest{
		OperationID: testLifecycleOperationID(t, "scrum-channel-replan", job.ID),
		ProjectID:   project.ID,
		CardID:      card.ID,
		Message:     "Replace the pending work once.",
	}
	command := ScrumChannelOperationCommand{
		Request:               request,
		ExpectedCardUpdatedAt: card.UpdatedAt,
		Effect:                ScrumChannelEffect{Kind: ScrumChannelReplanJob, JobID: job.ID},
		ResultAction:          "revised",
		ResultAgent:           "omnidex",
	}
	builder := func(current DBScrumCard, resultJob model.Job) (ScrumChannelCardUpdate, error) {
		return scrumChannelTestUpdate(t, current, request, resultJob), nil
	}
	first, err := repository.ExecuteScrumChannelOperation(ctx, command, builder)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.ExecuteScrumChannelOperation(ctx, command, builder)
	if err != nil {
		t.Fatal(err)
	}
	if first.Job.CurrentGeneration != 2 || second.Job.CurrentGeneration != 2 || second.Applied {
		t.Fatalf("replan results first=%+v second=%+v", first.Job, second.Job)
	}
	assertScrumChannelOperationCounts(t, pool, project.ID, card.ID, 1, 1, 1)
	var lifecycleRows int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM job_lifecycle_operations WHERE job_id=$1 AND kind='replan_job'
	`, job.ID).Scan(&lifecycleRows); err != nil {
		t.Fatal(err)
	}
	if lifecycleRows != 1 {
		t.Fatalf("job lifecycle rows=%d want 1", lifecycleRows)
	}
}

func TestPostgresScrumChannelOperationSubmitsFeedbackOnce(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	project, card := newScrumChannelOperationCard(t, repository, "feedback")
	job, err := repository.EnqueueJob(
		ctx,
		"Initial waiting Scrum channel job.",
		model.PipelineAssistant,
		[]byte(fmt.Sprintf(`{"project_id":%d}`, project.ID)),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, fmt.Sprintf("scrum-channel-feedback-%d", job.ID))
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v want job %d", claim, job.ID)
	}
	if err := repository.PauseStepForInput(ctx, claim.Authority, "waiting", "Continue?", nil); err != nil {
		t.Fatal(err)
	}
	card, err = repository.UpdateScrumCard(ctx, project.ID, card.ID, map[string]any{
		"job_id": fmt.Sprintf("%d", job.ID), "play_state": "running", "column": "in_progress",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := ScrumChannelOperationRequest{
		OperationID: testLifecycleOperationID(t, "scrum-channel-feedback", job.ID),
		ProjectID:   project.ID, CardID: card.ID, Message: "Continue exactly once.",
	}
	command := ScrumChannelOperationCommand{
		Request: request, ExpectedCardUpdatedAt: card.UpdatedAt,
		Effect:       ScrumChannelEffect{Kind: ScrumChannelSubmitFeedback, JobID: job.ID},
		ResultAction: "feedback", ResultAgent: "omnidex",
	}
	builder := func(current DBScrumCard, resultJob model.Job) (ScrumChannelCardUpdate, error) {
		return scrumChannelTestUpdate(t, current, request, resultJob), nil
	}
	first, err := repository.ExecuteScrumChannelOperation(ctx, command, builder)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.ExecuteScrumChannelOperation(ctx, command, builder)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Applied || second.Applied || first.Job.Status != model.JobStatusRunning || second.Job.Status != first.Job.Status {
		t.Fatalf("feedback results first=%+v second=%+v", first, second)
	}
	assertScrumChannelOperationCounts(t, pool, project.ID, card.ID, 1, 1, 1)
	var lifecycleRows int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM job_lifecycle_operations WHERE job_id=$1 AND kind='submit_feedback'
	`, job.ID).Scan(&lifecycleRows); err != nil {
		t.Fatal(err)
	}
	if lifecycleRows != 1 {
		t.Fatalf("feedback lifecycle rows=%d want 1", lifecycleRows)
	}
}

func newScrumChannelOperationCard(t *testing.T, repository *Repository, label string) (model.Project, DBScrumCard) {
	t.Helper()
	project, err := repository.CreateProject(
		t.Context(), fmt.Sprintf("scrum-channel-%s-%d", label, time.Now().UnixNano()), t.TempDir(), "", "", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(
		t.Context(), project.ID, "", "Channel operation", "", "assigned", nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return project, card
}

func scrumChannelTestUpdate(
	t *testing.T,
	current DBScrumCard,
	request ScrumChannelOperationRequest,
	job model.Job,
) ScrumChannelCardUpdate {
	t.Helper()
	var chat []map[string]any
	if err := json.Unmarshal(current.Chat, &chat); err != nil {
		t.Fatal(err)
	}
	chat = append(chat, map[string]any{
		"id": "message-" + string(request.OperationID), "role": "user",
		"content": request.Message, "created_at": "2026-08-09T00:00:00Z",
		"operation_id": request.OperationID,
	})
	raw, err := json.Marshal(chat)
	if err != nil {
		t.Fatal(err)
	}
	return ScrumChannelCardUpdate{
		Chat: raw, Column: "in_progress", JobID: fmt.Sprintf("%d", job.ID),
		PlayState: "running", QueueOrder: 0,
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
			(SELECT jsonb_array_length(chat) FROM scrum_cards WHERE project_id=$1 AND id=$2),
			(SELECT COUNT(*) FROM jobs WHERE project_id=$1)
	`, projectID, cardID).Scan(&operations, &messages, &jobs); err != nil {
		t.Fatal(err)
	}
	if operations != wantOperations || messages != wantMessages || jobs != wantJobs {
		t.Fatalf("counts operations=%d messages=%d jobs=%d", operations, messages, jobs)
	}
}
