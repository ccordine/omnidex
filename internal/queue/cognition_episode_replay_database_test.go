package queue

import (
	"errors"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresCognitionStartReplaySurvivesProgressAndAttemptReplacement(t *testing.T) {
	repository, pool, _ := policyInputFreshRepository(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	missingActivation := fixture.Start
	missingActivation.ProviderProcessActivation = cognitionpolicy.ProviderProcessActivation{}
	if _, err := repository.StartCognitionEpisode(
		t.Context(), missingActivation, cognitionTestFactAuthority(),
	); err == nil {
		t.Fatal("episode start accepted no provider process activation")
	}
	if _, err := repository.StartCognitionEpisode(t.Context(), fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	action, _ := prepareCognitionProposalAction(t, fixture)
	if _, err := repository.IngestCognitionTransition(
		t.Context(), fixture.Authority, action.Action.ID,
		cognitionProposalTransition(t, fixture, action), cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
	replay := fixture.Start
	replay.Authority = replacement
	replay.BrainBootstrap = freshReplayBrainBootstrap(t, fixture.Start.BrainBootstrap)
	replay.ProviderProcessActivation = cognitionGuardProviderProcessActivationFor(
		t, t.Context(), fixture.EpisodeID, replacement,
		replay.BrainBootstrap.AttestedBrain,
	)
	episode, err := repository.StartCognitionEpisode(t.Context(), replay, cognitionTestFactAuthority())
	if err != nil {
		t.Fatal(err)
	}
	if episode.EpisodeID != fixture.EpisodeID || episode.Authority != fixture.Authority {
		t.Fatalf("start replay changed immutable origin: %+v", episode)
	}
	var transitions, graphs int
	if err := pool.QueryRow(t.Context(), `
		SELECT (SELECT COUNT(*) FROM cognition_transitions WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_obligation_graphs WHERE episode_id=$1)
	`, fixture.EpisodeID).Scan(&transitions, &graphs); err != nil {
		t.Fatal(err)
	}
	if transitions != 2 || graphs != 2 {
		t.Fatalf("exact replay mutated transitions/graphs: %d/%d", transitions, graphs)
	}
	page, err := repository.ReadCognitionProviderProcessObservationPage(
		t.Context(), fixture.EpisodeID, CognitionProviderProcessObservationPageRequest{
			Scope: CognitionProviderObservationTerminalTrace,
			Limit: MaxCognitionProviderObservationPageSize,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 2 ||
		page.Records[0].Activation.Receipt.Actor != cognitionAttempt(fixture.Authority) ||
		page.Records[1].Activation.Receipt.Actor != cognitionAttempt(replacement) ||
		len(page.Records[0].Activation.IdentityEvidence.Operations) != 5 ||
		len(page.Records[1].Activation.IdentityEvidence.Operations) != 5 {
		t.Fatalf("start/replay provider activations=%+v", page.Records)
	}
	changed := replay
	extra, err := cognition.NewObservation(
		"observation-changed-start", changed.Transition.Current,
		"public_state", "Changed immutable start content.",
	)
	if err != nil {
		t.Fatal(err)
	}
	changed.Transition.Observations = append(
		append([]cognition.Observation(nil), changed.Transition.Observations...), extra,
	)
	if _, err := repository.StartCognitionEpisode(t.Context(), changed, cognitionTestFactAuthority()); !errors.Is(err, ErrCognitionConflict) {
		t.Fatalf("changed start replay error=%v, want ErrCognitionConflict", err)
	}
	changedBrain := replay
	changedBrain.BrainBootstrap = cognitionTestBrainBootstrapWithCPU("e")
	if _, err := repository.StartCognitionEpisode(
		t.Context(), changedBrain, cognitionTestFactAuthority(),
	); !errors.Is(err, cognitionpolicy.ErrInvalidEvidence) {
		t.Fatalf("changed attested brain replay error=%v, want invalid activation evidence", err)
	}
}

func freshReplayBrainBootstrap(
	t *testing.T,
	original cognitionpolicy.BrainBootstrap,
) cognitionpolicy.BrainBootstrap {
	t.Helper()
	request, err := cognitionpolicy.BootstrapProviderIdentityRequest(original.AttestedBrain.Ref)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := queueTestObservedProviderIdentity(
		time.Now().UTC().Truncate(time.Microsecond),
		original.AttestedBrain.Attestation, request.ChallengeSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	brain, err := cognitionpolicy.NewAttestedBrain(
		original.AttestedBrain.Ref, original.AttestedBrain.Attestation,
		observed.Observation, original.AttestedBrain.Host,
	)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := cognitionpolicy.NewBrainBootstrap(brain, observed.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	return bootstrap
}

func replaceCognitionAttemptForTest(
	t *testing.T,
	pool *pgxpool.Pool,
	old model.StepAttemptAuthority,
) model.StepAttemptAuthority {
	t.Helper()
	ctx := t.Context()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if result, err := tx.Exec(ctx, `
		UPDATE job_step_attempts SET status='expired',finished_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
		  AND worker_id=$5 AND status='active'
	`, old.JobID, old.Generation, old.StepID, old.Attempt, old.WorkerID); err != nil {
		t.Fatal(err)
	} else if result.RowsAffected() != 1 {
		t.Fatal("test replacement did not expire exactly one cognition attempt")
	}
	replacement := model.StepAttemptAuthority{
		JobID: old.JobID, Generation: old.Generation, StepID: old.StepID,
		Attempt: old.Attempt + 1, WorkerID: "cognition-replacement",
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_step_attempts (
			job_id,generation,step_id,attempt,worker_id,claimed_at,renewed_at
		) VALUES ($1,$2,$3,$4,$5,clock_timestamp(),clock_timestamp())
	`, replacement.JobID, replacement.Generation, replacement.StepID,
		replacement.Attempt, replacement.WorkerID); err != nil {
		t.Fatal(err)
	}
	if result, err := tx.Exec(ctx, `
		UPDATE job_steps SET current_attempt=$4,worker_id=$5,updated_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND id=$3 AND current_attempt=$6 AND status='running'
	`, replacement.JobID, replacement.Generation, replacement.StepID,
		replacement.Attempt, replacement.WorkerID, old.Attempt); err != nil {
		t.Fatal(err)
	} else if result.RowsAffected() != 1 {
		t.Fatal("test replacement did not bind exactly one cognition step")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return replacement
}
