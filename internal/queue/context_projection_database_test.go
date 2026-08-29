package queue

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresContextProjectionRoundTripBindingAndImmutability(t *testing.T) {
	ctx, repository, pool := openWorkingSetDatabase(t)
	marker := fmt.Sprintf("context-projection-%d", time.Now().UnixNano())
	authority, projection, stationJob := seedContextProjectionTest(t, ctx, repository, pool, marker)

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
	loaded, err := repository.GetContextProjection(ctx, projection.ID)
	if err != nil || !reflect.DeepEqual(loaded, created) {
		t.Fatalf("loaded=%+v error=%v, want %+v", loaded, err, created)
	}
	page, err := repository.ListContextProjectionSummaries(ctx, authority.JobID, authority.Generation, 0, 1)
	if err != nil || len(page) != 1 || page[0].ProjectionID != projection.ID || page[0].WorkID != projection.WorkID {
		t.Fatalf("projection page=%+v error=%v", page, err)
	}

	bound := prepareSuccessfulStationEvidenceFixture(
		t, repository, authority.StepAttemptAuthority, stationJob,
		`{"schema":"omnidex.conversation-response.v1","text":"bound context"}`,
	)
	call := persistPreparedStationEvidenceFixture(t, repository, bound, projection.ID)
	if call.ContextProjectionID != projection.ID || call.JobGeneration != authority.Generation {
		t.Fatalf("bound call lost exact projection authority: %+v", call)
	}
	unbound := prepareSuccessfulStationEvidenceFixture(
		t, repository, authority.StepAttemptAuthority,
		newStationEvidenceJobForTest(t, marker+"-unbound"),
		`{"schema":"omnidex.conversation-response.v1","text":"unbound context"}`,
	)
	unboundCall := persistPreparedStationEvidenceFixture(t, repository, unbound, "")
	if unboundCall.ContextProjectionID != "" {
		t.Fatalf("unbound call received a synthesized projection: %+v", unboundCall)
	}
	assertContextProjectionDatabaseValidation(t, ctx, pool, created)
	assertContextProjectionImmutable(t, ctx, pool, projection.ID)
}

func TestPostgresContextProjectionStoresCodeOwnedLiveUsage(t *testing.T) {
	ctx, repository, pool := openWorkingSetDatabase(t)
	marker := fmt.Sprintf("context-projection-live-%d", time.Now().UnixNano())
	authority, projection, _ := seedContextProjectionTest(t, ctx, repository, pool, marker)
	created, err := repository.StoreContextProjection(ctx, authority, projection)
	if err != nil {
		t.Fatal(err)
	}
	var usageMode string
	if err := pool.QueryRow(ctx, `
		SELECT usage_mode FROM context_projections WHERE projection_id=$1
	`, projection.ID).Scan(&usageMode); err != nil {
		t.Fatal(err)
	}
	if usageMode != "live" {
		t.Fatalf("stored context usage mode=%q want live", usageMode)
	}
	loaded, err := repository.GetContextProjection(ctx, projection.ID)
	if err != nil || !reflect.DeepEqual(loaded, created) {
		t.Fatalf("loaded live projection=%+v error=%v want=%+v", loaded, err, created)
	}
}

func TestPostgresContextProjectionBindingRejectsEveryAuthorityMismatch(t *testing.T) {
	ctx, repository, pool := openWorkingSetDatabase(t)
	marker := fmt.Sprintf("context-projection-authority-%d", time.Now().UnixNano())
	authority, projection, stationJob := seedContextProjectionTest(t, ctx, repository, pool, marker)
	if _, err := repository.StoreContextProjection(ctx, authority, projection); err != nil {
		t.Fatal(err)
	}
	fixture := prepareSuccessfulStationEvidenceFixture(
		t, repository, authority.StepAttemptAuthority, stationJob,
		`{"schema":"omnidex.conversation-response.v1","text":"authority binding"}`,
	)

	wrongWork := fixture.Record
	wrongWork.ContextProjectionID = projection.ID
	wrongWork.WorkID = strings.Repeat("f", 64)
	if _, err := insertLLMCallEvidenceForTest(ctx, repository, wrongWork); err == nil {
		t.Fatal("cross-work projection binding succeeded")
	}
	wrongKind := fixture.Record
	wrongKind.ContextProjectionID = projection.ID
	wrongKind.WorkKind = "other_work"
	if _, err := insertLLMCallEvidenceForTest(ctx, repository, wrongKind); err == nil {
		t.Fatal("cross-work-kind projection binding succeeded")
	}

	var secondStep int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO job_steps (job_id, action, sort_index, status, generation)
		VALUES ($1, 'projection_second_step', 99, 'pending', 1) RETURNING id
	`, authority.JobID).Scan(&secondStep); err != nil {
		t.Fatal(err)
	}
	wrongStep := fixture.Record
	wrongStep.ContextProjectionID, wrongStep.StepID = projection.ID, secondStep
	if _, err := insertLLMCallEvidenceForTest(ctx, repository, wrongStep); err == nil {
		t.Fatal("cross-step projection binding succeeded")
	}

	other := enqueueWorkingSetTestJob(t, ctx, repository, marker+"-other")
	otherAuthority := claimWorkingSetTestJob(t, ctx, repository, other)
	wrongJob := fixture.Record
	wrongJob.ContextProjectionID, wrongJob.StepID = projection.ID, otherAuthority.StepID
	if _, err := insertLLMCallEvidenceForTest(ctx, repository, wrongJob); err == nil {
		t.Fatal("cross-job projection binding succeeded")
	}

	persistPreparedStationEvidenceFixture(t, repository, fixture, projection.ID)
	secondGenerationStep := advanceContextProjectionGeneration(t, ctx, pool, authority.JobID)
	wrongGeneration := fixture.Record
	wrongGeneration.ContextProjectionID, wrongGeneration.StepID = projection.ID, secondGenerationStep
	if _, err := insertLLMCallEvidenceForTest(ctx, repository, wrongGeneration); err == nil {
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
) (ContextProjectionAuthority, contextbuilder.Projection, assemblyline.PortableJob) {
	t.Helper()
	job := enqueueWorkingSetTestJob(t, ctx, repository, marker)
	attemptAuthority := claimWorkingSetTestJob(t, ctx, repository, job)
	stationJob := newStationEvidenceJobForTest(t, marker+"-bound")
	authority, projection := seedContextProjectionForAuthorityWithWork(
		t, ctx, repository, job.ID, attemptAuthority, marker,
		stationJob.ID, string(stationJob.Kind),
	)
	return authority, projection, stationJob
}

func seedPreInlineExecutionContextProjectionTest(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	pool *pgxpool.Pool,
	marker string,
) (ContextProjectionAuthority, contextbuilder.Projection) {
	t.Helper()
	claim := seedPreInlineExecutionMigrationClaim(t, ctx, pool, marker)
	return seedContextProjectionForAuthority(
		t, ctx, repository, claim.Job.ID, claim.Authority, marker,
	)
}

func seedContextProjectionForAuthority(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	jobID int64,
	attemptAuthority model.StepAttemptAuthority,
	marker string,
) (ContextProjectionAuthority, contextbuilder.Projection) {
	t.Helper()
	return seedContextProjectionForAuthorityWithWork(
		t, ctx, repository, jobID, attemptAuthority, marker,
		llmEvidenceSHA256(marker), "repository_investigation",
	)
}

func seedContextProjectionForAuthorityWithWork(
	t *testing.T,
	ctx context.Context,
	repository *Repository,
	jobID int64,
	attemptAuthority model.StepAttemptAuthority,
	marker string,
	workID string,
	workKind string,
) (ContextProjectionAuthority, contextbuilder.Projection) {
	t.Helper()
	budget := workingset.Budget{MaxItems: 2, MaxBytes: 4096}
	created, err := repository.CreateCurrentWorkingSet(ctx, attemptAuthority, budget)
	if err != nil {
		t.Fatal(err)
	}
	request := workingSetDatabaseRequest("projection-item", created.Scope)
	commandID := workingSetDatabaseCommandID(t, marker, "acquire")
	if _, err := repository.ApplyWorkingSetCommand(ctx, attemptAuthority, workingset.AcquireCommand{
		CommandID: commandID, ExpectedVersion: 0, Actor: taskstate.AuthorityCode, Request: request,
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.CurrentWorkingSet(ctx, jobID)
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
	projection, err := contextbuilder.Build(contextbuilder.BuildInput{
		WorkID: workID,
		Spec: contextbuilder.ContextSpec{
			Name: "repository-investigation", Version: "v1",
			ScopeRef: taskstate.Ref{
				URI: fmt.Sprintf("task:job/%d/node/investigation", jobID), Version: "v1",
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
			SourceRefs: []taskstate.Ref{item.Ref},
			Content:    "evidence", ByteCost: len("evidence"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ContextProjectionAuthority{
		StepAttemptAuthority: attemptAuthority, WorkKind: workKind,
	}, projection
}
