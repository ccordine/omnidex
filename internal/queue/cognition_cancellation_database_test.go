package queue

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestPostgresCognitionCancellationSealsExactEvidenceAndReplays(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "typed-cancellation")
	command := cognitionCancellationForTest(t, fixture, errors.New("bounded policy schema rejection"))
	seal, err := fixture.Repository.CancelCognitionEpisode(fixture.Context, command)
	if err != nil {
		t.Fatal(err)
	}
	if seal.Code != command.Code || seal.SourceEvidenceID != command.SourceEvidence.ID {
		t.Fatalf("cancellation seal=%+v", seal)
	}
	replay, err := fixture.Repository.CancelCognitionEpisode(fixture.Context, command)
	if err != nil || replay != seal {
		t.Fatalf("cancellation replay=%+v error=%v", replay, err)
	}
	changed := cognitionCancellationForTest(t, fixture, errors.New("different policy schema rejection"))
	if _, err := fixture.Repository.CancelCognitionEpisode(
		fixture.Context, changed,
	); !errors.Is(err, ErrCognitionConflict) {
		t.Fatalf("changed cancellation replay error=%v, want conflict", err)
	}
	var status, traceJSON string
	var canceledNodes, cancellationRows int
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT episodes.status,seals.trace_json,
		       (SELECT COUNT(*) FROM task_nodes WHERE ledger_id=episodes.ledger_id AND status='canceled'),
		       (SELECT COUNT(*) FROM cognition_episode_cancellations WHERE episode_id=episodes.episode_id)
		FROM cognition_episodes episodes
		JOIN cognition_terminal_seals seals ON seals.episode_id=episodes.episode_id
		WHERE episodes.episode_id=$1
	`, fixture.EpisodeID).Scan(&status, &traceJSON, &canceledNodes, &cancellationRows); err != nil {
		t.Fatal(err)
	}
	if status != string(CognitionEpisodeCanceled) || canceledNodes == 0 || cancellationRows != 1 ||
		!strings.Contains(traceJSON, command.SourceEvidence.ID) {
		t.Fatalf("status/nodes/rows/trace=%q/%d/%d/%t", status, canceledNodes, cancellationRows,
			strings.Contains(traceJSON, command.SourceEvidence.ID))
	}
	page, err := fixture.Repository.ReadCognitionSealedTrace(
		fixture.Context, fixture.EpisodeID, CognitionTracePageRequest{Limit: MaxCognitionTracePageSize},
	)
	if err != nil {
		t.Fatalf("read canceled sealed trace: %v", err)
	}
	seenCancellation := false
	for _, record := range page.Records {
		if record.Kind == "cancellation_evidence" && record.ID == command.SourceEvidence.ID {
			seenCancellation = true
		}
	}
	if !seenCancellation {
		t.Fatalf("canceled sealed trace omitted evidence %q", command.SourceEvidence.ID)
	}
}

func TestPostgresCognitionCancellationRejectsUnresolvedAction(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "cancellation-unresolved")
	_ = prepareCognitionGuardAction(t, fixture, "cancellation-unresolved")
	command := cognitionCancellationForTest(t, fixture, errors.New("bounded policy failure"))
	if _, err := fixture.Repository.CancelCognitionEpisode(
		fixture.Context, command,
	); !errors.Is(err, ErrCognitionConflict) {
		t.Fatalf("unresolved cancellation error=%v, want conflict", err)
	}
	var count int
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT COUNT(*) FROM cognition_episode_cancellations WHERE episode_id=$1
	`, fixture.EpisodeID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cancellation rows=%d error=%v", count, err)
	}
}

func TestPostgresCognitionCancellationEvidenceIdentityIsEpisodeScoped(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	left := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "cancellation-evidence-left",
	)
	right := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "cancellation-evidence-right",
	)
	leftCommand := cognitionCancellationForTest(t, left, errors.New("same bounded policy failure"))
	rightCommand := cognitionCancellationForTest(t, right, errors.New("same bounded policy failure"))
	if leftCommand.SourceEvidence.ID != rightCommand.SourceEvidence.ID {
		t.Fatalf("same failure evidence identity changed: %q != %q",
			leftCommand.SourceEvidence.ID, rightCommand.SourceEvidence.ID)
	}
	if _, err := left.Repository.CancelCognitionEpisode(left.Context, leftCommand); err != nil {
		t.Fatalf("cancel left episode: %v", err)
	}
	if _, err := right.Repository.CancelCognitionEpisode(right.Context, rightCommand); err != nil {
		t.Fatalf("cancel right episode with same evidence identity: %v", err)
	}
	var count int
	if err := left.Pool.QueryRow(left.Context, `
		SELECT COUNT(*) FROM cognition_episode_cancellations
		WHERE source_evidence_id=$1 AND episode_id IN ($2,$3)
	`, leftCommand.SourceEvidence.ID, left.EpisodeID, right.EpisodeID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("episode-scoped evidence rows=%d error=%v", count, err)
	}
}

func TestPostgresCognitionCancellationRejectsForeignActorAndOrphan(t *testing.T) {
	left := startTaskGenerationRetirementFixture(t, "cancellation-left")
	right := startTaskGenerationRetirementFixture(t, "cancellation-right")
	command := cognitionCancellationForTest(t, left, errors.New("bounded policy failure"))
	raw, jsonSHA, err := cognitionJSON(command.SourceEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := left.Pool.Exec(left.Context, cognitionCancellationInsertSQL,
		left.EpisodeID, command.Code, int64(command.ExpectedRevision.Number), command.ExpectedRevision.SHA256,
		command.SourceEvidence.ID, string(raw), command.SourceEvidence.SHA256, jsonSHA,
		right.Authority.JobID, right.Authority.Generation, right.Authority.StepID,
		right.Authority.Attempt, right.Authority.WorkerID); err == nil {
		t.Fatal("foreign cognition cancellation actor was accepted")
	}
	tx, err := left.Pool.Begin(left.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(left.Context)
	if _, err := tx.Exec(left.Context, cognitionCancellationInsertSQL,
		left.EpisodeID, command.Code, int64(command.ExpectedRevision.Number), command.ExpectedRevision.SHA256,
		command.SourceEvidence.ID, string(raw), command.SourceEvidence.SHA256, jsonSHA,
		left.Authority.JobID, left.Authority.Generation, left.Authority.StepID,
		left.Authority.Attempt, left.Authority.WorkerID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(left.Context); err == nil {
		t.Fatal("orphan cognition cancellation without canceled seal was accepted")
	}
}

func cognitionCancellationForTest(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	source error,
) cognitionruntime.CancellationCommand {
	t.Helper()
	evidence, err := cognitionruntime.NewCancellationEvidence(
		cognitionruntime.CancellationPolicyFailure,
		"The bounded cognition policy response was rejected.", source,
	)
	if err != nil {
		t.Fatal(err)
	}
	return cognitionruntime.CancellationCommand{
		Binding: cognitionruntime.Binding{
			Episode: cognition.EpisodeRef{ID: fixture.EpisodeID},
			Attempt: cognition.AttemptRef{
				JobID: fixture.Authority.JobID, Generation: fixture.Authority.Generation,
				StepID: fixture.Authority.StepID, Attempt: uint64(fixture.Authority.Attempt),
				WorkerID: fixture.Authority.WorkerID,
			},
		},
		ExpectedRevision: fixture.Start.Transition.Current,
		Code:             cognitionruntime.CancellationPolicyFailure, SourceEvidence: evidence,
	}
}

const cognitionCancellationInsertSQL = `
	INSERT INTO cognition_episode_cancellations (
		episode_id,cancellation_code,expected_revision,expected_revision_sha256,
		source_evidence_id,source_evidence_json,source_evidence_sha256,source_evidence_json_sha256,
		job_id,generation,step_id,authority_kind,actor_attempt,actor_worker_id,lifecycle_operation_id
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'worker',$12,$13,NULL)
`
