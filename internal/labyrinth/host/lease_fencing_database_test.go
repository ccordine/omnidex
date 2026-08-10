package host

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
)

func TestPostgresApplyRollsBackWhenLeaseExpiresDuringKernelWork(t *testing.T) {
	fixture := newFencedHostFixture(t)
	authority := fixture.installExpiringAttempt(t, "host-old-worker")
	actor := attemptRef(authority)
	resolver := func(_ context.Context, reference cognition.ScenarioRef) (labyrinth.Scenario, error) {
		if reference != fixture.scenario.Ref() {
			return labyrinth.Scenario{}, cognition.ErrInvalidScenario
		}
		return fixture.scenario, nil
	}
	precheck := func(ctx context.Context, candidate cognition.AttemptRef) error {
		return fixture.repository.AuthorizeStepAttempt(ctx, stepAuthority(candidate))
	}
	transactional := func(ctx context.Context, tx pgx.Tx, candidate cognition.AttemptRef) error {
		return fixture.repository.AuthorizeStepAttemptTransaction(ctx, tx, stepAuthority(candidate))
	}
	environment, err := NewEnvironment(
		fixture.store, fixture.episode, resolver, precheck, transactional,
	)
	if err != nil {
		t.Fatal(err)
	}
	started, err := environment.Start(t.Context(), fixture.scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	action := fencedWitnessAction(t, fixture, started, actor)

	base := environment.newKernel
	entered, release := make(chan struct{}), make(chan struct{})
	blocker := &blockingReplayAuthority{entered: entered, release: release}
	environment.newKernel = func(
		scenario labyrinth.Scenario,
		episode cognition.EpisodeRef,
		authorize labyrinth.AttemptAuthorizer,
	) (kernelCandidate, error) {
		candidate, buildErr := base(scenario, episode, authorize)
		if buildErr != nil {
			return kernelCandidate{}, buildErr
		}
		return kernelCandidate{
			replayKernel: &blockingReplayKernel{
				replayKernel: candidate.replayKernel, authority: blocker,
			},
			close: candidate.close,
		}, nil
	}

	oldResult := make(chan error, 1)
	go func() {
		_, applyErr := environment.Apply(
			context.Background(), fixture.episode, started.Current, action,
		)
		oldResult <- applyErr
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("old host apply did not reach blocked kernel work")
	}
	var expiresAt time.Time
	if err := fixture.pool.QueryRow(t.Context(), `
		SELECT expires_at FROM job_step_attempts
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4
	`, authority.JobID, authority.Generation, authority.StepID, authority.Attempt).Scan(&expiresAt); err != nil {
		t.Fatal(err)
	}
	if wait := time.Until(expiresAt) + 50*time.Millisecond; wait > 0 {
		timer := time.NewTimer(wait)
		<-timer.C
	}
	lockedClaim, err := fixture.repository.ClaimNextStep(t.Context(), "host-replacement-worker-early")
	if err != nil {
		t.Fatal(err)
	}
	if lockedClaim != nil {
		t.Fatalf("replacement claimed while old host transaction held authority: %+v", lockedClaim)
	}
	close(release)
	if applyErr := <-oldResult; !errors.Is(applyErr, cognition.ErrAuthorityDenied) {
		t.Fatalf("expired old apply error=%v, want authority denial", applyErr)
	}
	claim, err := fixture.repository.ClaimNextStep(t.Context(), "host-replacement-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Authority.JobID != authority.JobID ||
		claim.Authority.StepID != authority.StepID || claim.Authority.Attempt != 2 ||
		claim.Authority.WorkerID != "host-replacement-worker" {
		t.Fatalf("replacement claim=%+v", claim)
	}
	assertHostHead(t, fixture, started.Current, 0)

	replacement := action.Clone()
	replacement.Actor = attemptRef(claim.Authority)
	transition, err := environment.Apply(
		t.Context(), fixture.episode, started.Current, replacement,
	)
	if err != nil {
		t.Fatal(err)
	}
	if transition.Current.Number != started.Current.Number+1 {
		t.Fatalf("replacement revision=%d", transition.Current.Number)
	}
	if _, err := environment.Apply(
		t.Context(), fixture.episode, started.Current, action,
	); !errors.Is(err, cognition.ErrAuthorityDenied) {
		t.Fatalf("stale old actor replay error=%v", err)
	}
	assertHostHead(t, fixture, transition.Current, 1)
}

func TestPostgresLifecycleRetirementFencesApplyAfterStandaloneAuthorization(t *testing.T) {
	fixture := newFencedHostFixture(t)
	authority := fixture.installActiveAttempt(t, "host-retired-worker")
	actor := attemptRef(authority)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	precheck := func(ctx context.Context, candidate cognition.AttemptRef) error {
		if err := fixture.repository.AuthorizeStepAttempt(ctx, stepAuthority(candidate)); err != nil {
			return err
		}
		once.Do(func() {
			close(entered)
			<-release
		})
		return nil
	}
	transactional := func(ctx context.Context, tx pgx.Tx, candidate cognition.AttemptRef) error {
		return fixture.repository.AuthorizeStepAttemptTransaction(ctx, tx, stepAuthority(candidate))
	}
	environment := fixture.newEnvironment(t, precheck, transactional)
	started, err := environment.Start(t.Context(), fixture.scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	fixture.startCognitionEpisode(t, authority, started)
	action := fencedWitnessAction(t, fixture, started, actor)

	applyResult := make(chan error, 1)
	go func() {
		_, applyErr := environment.Apply(
			context.Background(), fixture.episode, started.Current, action,
		)
		applyResult <- applyErr
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("host apply did not pause after standalone authorization")
	}
	operationID, err := queue.NewLifecycleOperationID(
		"labyrinth-host-test", "cancel-before-host-transaction", fmt.Sprint(authority.JobID),
	)
	if err != nil {
		t.Fatal(err)
	}
	canceled, err := fixture.repository.CancelJob(t.Context(), queue.CancelJobCommand{
		OperationID: operationID, JobID: authority.JobID,
		Reason: "Retire the active cognition attempt before host mutation.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if canceled.Status != model.JobStatusCanceled {
		t.Fatalf("canceled job status=%q", canceled.Status)
	}
	episode, err := fixture.repository.CognitionEpisode(t.Context(), fixture.episode.ID)
	if err != nil {
		t.Fatal(err)
	}
	if episode.Status != queue.CognitionEpisodeCanceled {
		t.Fatalf("retired cognition episode status=%q", episode.Status)
	}

	close(release)
	if applyErr := <-applyResult; !errors.Is(applyErr, cognition.ErrAuthorityDenied) {
		t.Fatalf("retired host apply error=%v, want authority denial", applyErr)
	}
	assertHostHead(t, fixture, started.Current, 0)
	if _, err := environment.Apply(
		t.Context(), fixture.episode, started.Current, action,
	); !errors.Is(err, cognition.ErrAuthorityDenied) {
		t.Fatalf("retired actor replay error=%v", err)
	}
	assertHostHead(t, fixture, started.Current, 0)
}

type blockingReplayAuthority struct {
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

type blockingReplayKernel struct {
	replayKernel
	authority *blockingReplayAuthority
}

func (kernel *blockingReplayKernel) Apply(
	ctx context.Context,
	episode cognition.EpisodeRef,
	expected cognition.WorldRevision,
	action cognition.RegisteredAction,
) (cognition.Transition, error) {
	kernel.authority.once.Do(func() {
		close(kernel.authority.entered)
		<-kernel.authority.release
	})
	return kernel.replayKernel.Apply(ctx, episode, expected, action)
}
