package queue

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresContextProjectionRoundTripBindingAndImmutability(t *testing.T) {
	ctx, repository, pool := openWorkingSetDatabase(t)
	marker := fmt.Sprintf("context-projection-%d", time.Now().UnixNano())
	authority, projection := seedContextProjectionTest(t, ctx, repository, pool, marker)

	created, err := repository.StoreContextProjection(ctx, authority, projection)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := repository.StoreContextProjection(ctx, authority, projection)
	if err != nil || !reflect.DeepEqual(replayed, created) {
		t.Fatalf("exact replay=%+v error=%v, want %+v", replayed, err, created)
	}
	changed := authority
	changed.WorkKind = "different_work"
	if _, err := repository.StoreContextProjection(ctx, changed, projection); !errors.Is(err, ErrContextProjectionConflict) {
		t.Fatalf("changed identity error=%v, want conflict", err)
	}
	changed = authority
	changed.Mode = ContextProjectionMode("applied")
	if _, err := repository.StoreContextProjection(ctx, changed, projection); !errors.Is(err, ErrInvalidContextProjection) {
		t.Fatalf("unsupported usage mode error=%v, want invalid projection", err)
	}
	loaded, err := repository.GetContextProjection(ctx, projection.ID)
	if err != nil || !reflect.DeepEqual(loaded, created) {
		t.Fatalf("loaded=%+v error=%v, want %+v", loaded, err, created)
	}
	page, err := repository.ListContextProjectionSummaries(ctx, authority.JobID, authority.Generation, 0, 1)
	if err != nil || len(page) != 1 || page[0].ProjectionID != projection.ID || page[0].WorkID != projection.WorkID {
		t.Fatalf("projection page=%+v error=%v", page, err)
	}

	bound := contextProjectionLLMRecord(authority, projection)
	bound.ContextProjectionID = projection.ID
	call, err := repository.RecordLLMCallEvidence(ctx, bound)
	if err != nil {
		t.Fatal(err)
	}
	if call.ContextProjectionID != projection.ID || call.JobGeneration != authority.Generation {
		t.Fatalf("bound call lost exact projection authority: %+v", call)
	}
	legacy := contextProjectionLLMRecord(authority, projection)
	legacy.Attempt = 2
	legacyCall, err := repository.RecordLLMCallEvidence(ctx, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if legacyCall.ContextProjectionID != "" {
		t.Fatalf("shadow-era null projection was synthesized: %+v", legacyCall)
	}
	assertContextProjectionDatabaseValidation(t, ctx, pool, created)
	assertContextProjectionImmutable(t, ctx, pool, projection.ID)
}

func TestPostgresContextProjectionBindingRejectsEveryAuthorityMismatch(t *testing.T) {
	ctx, repository, pool := openWorkingSetDatabase(t)
	marker := fmt.Sprintf("context-projection-authority-%d", time.Now().UnixNano())
	authority, projection := seedContextProjectionTest(t, ctx, repository, pool, marker)
	if _, err := repository.StoreContextProjection(ctx, authority, projection); err != nil {
		t.Fatal(err)
	}

	wrongWork := contextProjectionLLMRecord(authority, projection)
	wrongWork.ContextProjectionID = projection.ID
	wrongWork.WorkID = strings.Repeat("f", 64)
	if _, err := repository.RecordLLMCallEvidence(ctx, wrongWork); err == nil {
		t.Fatal("cross-work projection binding succeeded")
	}
	wrongKind := contextProjectionLLMRecord(authority, projection)
	wrongKind.ContextProjectionID = projection.ID
	wrongKind.WorkKind = "other_work"
	if _, err := repository.RecordLLMCallEvidence(ctx, wrongKind); err == nil {
		t.Fatal("cross-work-kind projection binding succeeded")
	}

	var secondStep int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO job_steps (job_id, action, sort_index, status, generation)
		VALUES ($1, 'projection_second_step', 99, 'running', 1) RETURNING id
	`, authority.JobID).Scan(&secondStep); err != nil {
		t.Fatal(err)
	}
	wrongStep := contextProjectionLLMRecord(authority, projection)
	wrongStep.ContextProjectionID, wrongStep.StepID = projection.ID, secondStep
	if _, err := repository.RecordLLMCallEvidence(ctx, wrongStep); err == nil {
		t.Fatal("cross-step projection binding succeeded")
	}

	other := enqueueWorkingSetTestJob(t, ctx, repository, marker+"-other")
	otherStep := startContextProjectionJob(t, ctx, pool, other.ID)
	wrongJob := contextProjectionLLMRecord(authority, projection)
	wrongJob.ContextProjectionID, wrongJob.StepID = projection.ID, otherStep
	if _, err := repository.RecordLLMCallEvidence(ctx, wrongJob); err == nil {
		t.Fatal("cross-job projection binding succeeded")
	}

	secondGenerationStep := advanceContextProjectionGeneration(t, ctx, pool, authority.JobID)
	wrongGeneration := contextProjectionLLMRecord(authority, projection)
	wrongGeneration.ContextProjectionID, wrongGeneration.StepID = projection.ID, secondGenerationStep
	if _, err := repository.RecordLLMCallEvidence(ctx, wrongGeneration); err == nil {
		t.Fatal("cross-generation projection binding succeeded")
	}
	if replay, err := repository.StoreContextProjection(ctx, authority, projection); err != nil || replay.Projection.ID != projection.ID {
		t.Fatalf("historical exact replay failed after generation advance: %+v error=%v", replay, err)
	}
}

func seedContextProjectionTest(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	pool *pgxpool.Pool,
	marker string,
) (ContextProjectionAuthority, contextbuilder.Projection) {
	t.Helper()
	job := enqueueWorkingSetTestJob(t, ctx, repository, marker)
	budget := workingset.Budget{MaxItems: 2, MaxBytes: 4096}
	created, err := repository.CreateCurrentWorkingSet(ctx, job.ID, 1, budget)
	if err != nil {
		t.Fatal(err)
	}
	request := workingSetDatabaseRequest("projection-item", created.Scope)
	commandID := workingSetDatabaseCommandID(t, marker, "acquire")
	if _, err := repository.ApplyWorkingSetCommand(ctx, job.ID, 1, workingset.AcquireCommand{
		CommandID: commandID, ExpectedVersion: 0, Actor: taskstate.AuthorityCode, Request: request,
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.CurrentWorkingSet(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	set, err := workingset.Restore(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	item, exists := set.Item(request.ID)
	if !exists {
		t.Fatal("acquired projection item is missing")
	}
	workID := llmEvidenceSHA256(marker)
	projection, err := contextbuilder.Build(contextbuilder.BuildInput{
		WorkID: workID,
		Spec: contextbuilder.ContextSpec{
			Name: "repository-investigation", Version: "v1",
			ScopeRef: taskstate.Ref{
				URI: fmt.Sprintf("task:job/%d/node/investigation", job.ID), Version: "v1",
				Hash: strings.Repeat("d", 64), Relation: taskstate.RefConcerns,
			},
			Required: []contextbuilder.Selector{{
				ID: "repository", Role: workingset.RoleRepositoryEvidence, MinItems: 1, MaxItems: 1,
			}},
			AllowedAuthorities: []taskstate.Authority{taskstate.AuthorityToolEvidence},
			MaxItems:           2, MaxBytes: 4096, MaxAcquisitionRounds: 1,
		},
		WorkingSet: set,
		Materials: []contextbuilder.Material{{
			ItemID: item.ID, CurrentRef: item.Ref, Authority: taskstate.AuthorityToolEvidence,
			Content: "evidence", ByteCost: len("evidence"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	stepID := startContextProjectionJob(t, ctx, pool, job.ID)
	return ContextProjectionAuthority{
		JobID: job.ID, Generation: 1, StepID: stepID, WorkKind: "repository_investigation",
		Mode: ContextProjectionModeShadow,
	}, projection
}

func startContextProjectionJob(t *testing.T, ctx context.Context, pool *pgxpool.Pool, jobID int64) int64 {
	t.Helper()
	var stepID int64
	if err := pool.QueryRow(ctx, `
		UPDATE job_steps SET status='running', started_at=NOW(), updated_at=NOW()
		WHERE id=(SELECT id FROM job_steps WHERE job_id=$1 ORDER BY sort_index,id LIMIT 1)
		RETURNING id
	`, jobID).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	if result, err := pool.Exec(ctx, `UPDATE jobs SET status=$2, updated_at=NOW() WHERE id=$1`, jobID, model.JobStatusRunning); err != nil || result.RowsAffected() != 1 {
		t.Fatalf("start context projection job rows=%d error=%v", result.RowsAffected(), err)
	}
	return stepID
}

func contextProjectionLLMRecord(
	authority ContextProjectionAuthority,
	projection contextbuilder.Projection,
) LLMCallEvidenceRecord {
	return LLMCallEvidenceRecord{
		StepID: authority.StepID, Scope: "context_projection_shadow",
		WorkID: projection.WorkID, WorkKind: authority.WorkKind,
		RequestedModel: "requested", Model: "effective", Attempt: 1,
		SystemPrompt: "exact system", UserPrompt: "exact user", ResponseFormat: "text",
		ContextTokens: 4096, MaxOutputTokens: 512,
		Status: LLMEvidenceSucceeded, Response: "exact response", LatencyMS: 1,
	}
}
