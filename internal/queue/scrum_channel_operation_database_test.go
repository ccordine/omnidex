package queue

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresScrumChannelOperationStartsOnceAndReplaysExactly(t *testing.T) {
	repository, pool, ctx := scrumChannelOperationTestRepository(t)
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
		},
		ResultAction: "started",
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
	if first.OperationID != request.OperationID || second.OperationID != request.OperationID ||
		first.Job.ID != second.Job.ID || first.Card.ID != second.Card.ID ||
		!first.Card.UpdatedAt.Equal(second.Card.UpdatedAt) || !reflect.DeepEqual(first.Messages, second.Messages) {
		t.Fatalf("replay mismatch first=%+v second=%+v", first, second)
	}
	assertScrumChannelOperationCounts(t, pool, project.ID, card.ID, 1, 1, 1)
	var dbOwnedCreatedAt bool
	if err := pool.QueryRow(ctx, `
		SELECT isfinite(created_at) AND created_at=date_trunc('microseconds',created_at)
		FROM scrum_channel_operations WHERE operation_id=$1
	`, request.OperationID).Scan(&dbOwnedCreatedAt); err != nil {
		t.Fatal(err)
	}
	if !dbOwnedCreatedAt {
		t.Fatal("runtime Scrum operation lacks a DB-owned microsecond timestamp")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO scrum_channel_operations(
		 operation_id,project_id,card_id,effect_kind,effect_operation_id,
		 job_id,result_action,created_at
		)
		SELECT operation_id,project_id,card_id,effect_kind,effect_operation_id,
		 job_id,result_action,TIMESTAMPTZ '2020-01-01T00:00:00Z'
		FROM scrum_channel_operations WHERE operation_id=$1
	`, request.OperationID); err == nil || !strings.Contains(err.Error(), "caller-supplied created_at") {
		t.Fatalf("forged operation timestamp error=%v", err)
	}
	if _, err := repository.ReplanJob(ctx, ReplanJobCommand{
		OperationID: request.OperationID, JobID: first.Job.ID,
		Feedback: "Attempt to reuse the submitted card identity as a direct replan.",
	}); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatalf("cross-kind lifecycle identity reuse error=%v", err)
	}

	changed := command
	changed.Request.Message = "Changed content under the same identity."
	changed.Effect.Instruction = changed.Request.Message
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
	repository, pool, ctx := scrumChannelOperationTestRepository(t)
	project, card := newScrumChannelOperationCard(t, repository, "replan")
	job := enqueueScrumChannelJobForTest(t, repository, card, "Initial Scrum channel job.")
	card = setScrumCardRunningForTest(t, pool, project.ID, card.ID, job.ID)
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
		ResultAction:          "replanned",
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
	repository, pool, ctx := scrumChannelOperationTestRepository(t)
	project, card := newScrumChannelOperationCard(t, repository, "feedback")
	job := enqueueScrumChannelJobForTest(t, repository, card, "Initial waiting Scrum channel job.")
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
	card = setScrumCardRunningForTest(t, pool, project.ID, card.ID, job.ID)
	request := ScrumChannelOperationRequest{
		OperationID: testLifecycleOperationID(t, "scrum-channel-feedback", job.ID),
		ProjectID:   project.ID, CardID: card.ID, Message: "Continue exactly once.",
	}
	command := ScrumChannelOperationCommand{
		Request: request, ExpectedCardUpdatedAt: card.UpdatedAt,
		Effect:       ScrumChannelEffect{Kind: ScrumChannelSubmitFeedback, JobID: job.ID},
		ResultAction: "feedback",
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
	if !first.Applied || second.Applied || second.Job.Status != first.Job.Status {
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

func TestPostgresScrumChannelReplayReturnsCurrentTruthAfterBoundMessageLeavesTail(t *testing.T) {
	repository, _, ctx := scrumChannelOperationTestRepository(t)
	project, card := newScrumChannelOperationCard(t, repository, "current-truth")
	request := ScrumChannelOperationRequest{
		OperationID: testLifecycleOperationID(t, "scrum-channel-current-truth", project.ID),
		ProjectID:   project.ID, CardID: card.ID, Message: "Start before the card changes.",
	}
	first, err := repository.ExecuteScrumChannelOperation(ctx, ScrumChannelOperationCommand{
		Request: request, ExpectedCardUpdatedAt: card.UpdatedAt,
		Effect:       ScrumChannelEffect{Kind: ScrumChannelStartJob, Instruction: request.Message},
		ResultAction: "started",
	}, func(current DBScrumCard, job model.Job) (ScrumChannelCardUpdate, error) {
		return scrumChannelTestUpdate(t, current, request, job), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	later := make([]ScrumCardMessageAppend, 55)
	for index := range later {
		later[index] = ScrumCardMessageAppend{
			ID: fmt.Sprintf("current-truth-later-%02d", index), Role: "assistant",
			Content: fmt.Sprintf("later authoritative message %02d", index),
		}
	}
	latest := appendScrumMessagesForTest(t, repository, project.ID, card.ID, later)
	paused, err := repository.PauseScrumCardPlayAtRevision(ctx, project.ID, card.ID, latest.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	replay, found, err := repository.LoadScrumChannelOperation(ctx, request)
	if err != nil || !found {
		t.Fatalf("current-truth replay found=%t error=%v", found, err)
	}
	for _, message := range replay.Messages {
		if message.OperationID == string(request.OperationID) {
			t.Fatal("current 50-row tail unexpectedly retained the original bound command")
		}
	}
	if replay.Applied || replay.OperationID != request.OperationID || replay.Action != "started" ||
		replay.Card.PlayState != "paused" || !replay.Card.UpdatedAt.Equal(paused.UpdatedAt) ||
		replay.Job.ID != first.Job.ID || replay.Job.Status != model.JobStatusCanceled ||
		len(replay.Messages) != 50 {
		t.Fatalf("current-truth replay=%+v messages=%d", replay, len(replay.Messages))
	}
}
