package cognitiongauntlet

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/labyrinth"
	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresFullCognitionSealsPolicyFailuresWithoutFallback(t *testing.T) {
	for _, test := range []struct {
		name     string
		response string
		failure  error
	}{
		{name: "rejected", response: `{"invalid":true}`},
		{name: "failed", failure: errors.New("registered provider generation failure")},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, pool, repository, hostStore := openFullCognitionDatabase(t)
			fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[0])
			if err != nil {
				t.Fatal(err)
			}
			claim := claimFailureStep(t, repository, fixture.Spec().Budget.WorkingSetBytes)
			generation := mustRatGeneration(t)
			client := &terminalPolicyClient{
				witnessPolicyClient: &witnessPolicyClient{model: generation.Fixed.Brain.Model},
				response:            test.response, failure: test.failure,
			}
			result, err := RunFullCognition(ctx, fixture, failureRunRequest(
				t, pool, hostStore, claim, generation, client,
			))
			if err != nil {
				t.Fatal(err)
			}
			assertCanceledFullCognition(t, pool, result, 1, client.calls())
		})
	}
}

func TestPostgresFullCognitionSealsCycleExhaustionWithoutFallback(t *testing.T) {
	ctx, pool, repository, hostStore := openFullCognitionDatabase(t)
	spec := InitialMicrogauntletsV1()[0]
	spec.Budget.RuntimeCycles = spec.Generator.Difficulty.SolutionDepth + 1
	fixture, err := GenerateMicrogauntlet(spec)
	if err != nil {
		t.Fatal(err)
	}
	claim := claimFailureStep(t, repository, fixture.Spec().Budget.WorkingSetBytes)
	generation := mustRatGeneration(t)
	first := fixture.generated.PrivateOracle().Witness[0]
	witness := make([]labyrinth.WitnessAction, spec.Budget.RuntimeCycles)
	for index := range witness {
		witness[index] = first
	}
	client := &witnessPolicyClient{model: generation.Fixed.Brain.Model, witness: witness}
	result, err := RunFullCognition(ctx, fixture, failureRunRequest(
		t, pool, hostStore, claim, generation, client,
	))
	if err != nil {
		t.Fatal(err)
	}
	if result.Episode.Manifest.Outcome.FailureCode !=
		string(cognitionruntime.CancellationRunBudgetExhausted) {
		t.Fatalf("failure code=%q", result.Episode.Manifest.Outcome.FailureCode)
	}
	assertCanceledFullCognition(t, pool, result, spec.Budget.RuntimeCycles, client.calls())
}

type terminalPolicyClient struct {
	*witnessPolicyClient
	mu       sync.Mutex
	count    int
	response string
	failure  error
}

func (client *terminalPolicyClient) GeneratePrepared(
	_ context.Context,
	prepared llm.PreparedModel,
) (string, error) {
	if err := llm.ValidateResponseContract(prepared); err != nil {
		return "", err
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	client.count++
	return client.response, client.failure
}

func (client *terminalPolicyClient) calls() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.count
}

func claimFailureStep(
	t *testing.T,
	repository *queue.Repository,
	workingSetBytes int,
) model.StepAttemptAuthority {
	t.Helper()
	job, err := repository.EnqueueJob(
		t.Context(), fmt.Sprintf("gauntlet-failure-%d", time.Now().UnixNano()),
		model.PipelineAssistant, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), fmt.Sprintf("gauntlet-failure-worker-%d", job.ID))
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim failure step=%+v error=%v", claim, err)
	}
	budget, err := fullCognitionWorkingSetBudget(workingSetBytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateCurrentWorkingSet(t.Context(), claim.Authority, budget); err != nil {
		t.Fatal(err)
	}
	return claim.Authority
}

func failureRunRequest(
	t *testing.T,
	pool *pgxpool.Pool,
	hostStore *labyrinthhost.Store,
	claim model.StepAttemptAuthority,
	generation RatGeneration,
	client llm.Client,
) FullCognitionRunRequest {
	t.Helper()
	episodeDirectory, evaluationDirectory := t.TempDir(), t.TempDir()
	return FullCognitionRunRequest{
		Surface: SurfaceSymbolic, RatGeneration: generation,
		RuntimeFingerprint: transferTestFingerprint(), Repetition: 1,
		Attempt: claim, Pool: pool, Client: client, HostStore: hostStore,
		EpisodeSealPath:     filepath.Join(episodeDirectory, "episode.json"),
		EvaluationPath:      filepath.Join(evaluationDirectory, "evaluation.json"),
		LedgerSchemaVersion: "task-ledger.v1", WorkingSetPolicyVersion: "working-set.v1",
		ProjectionPolicyVersion: "context-projection.v1",
	}
}

func assertCanceledFullCognition(
	t *testing.T,
	pool *pgxpool.Pool,
	result FullCognitionRunResult,
	wantCalls int,
	clientCalls int,
) {
	t.Helper()
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if result.Evaluation.GoalSuccess || result.Episode.Manifest.Outcome.GoalSatisfied ||
		result.Episode.Manifest.Outcome.FailureCode == "" || result.Evaluation.Attribution == nil {
		t.Fatalf("canceled result=%+v", result)
	}
	var status string
	var cancellations, seals, calls int
	if err := pool.QueryRow(t.Context(), `
		SELECT episodes.status,
		       (SELECT COUNT(*) FROM cognition_episode_cancellations WHERE episode_id=episodes.episode_id),
		       (SELECT COUNT(*) FROM cognition_terminal_seals WHERE episode_id=episodes.episode_id),
		       (SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=episodes.episode_id)
		FROM cognition_episodes episodes WHERE episodes.episode_id=$1
	`, result.Episode.Manifest.EpisodeID).Scan(&status, &cancellations, &seals, &calls); err != nil {
		t.Fatal(err)
	}
	if status != string(queue.CognitionEpisodeCanceled) || cancellations != 1 || seals != 1 ||
		calls != wantCalls || clientCalls != wantCalls {
		t.Fatalf("status/cancellations/seals/calls/client=%s/%d/%d/%d/%d",
			status, cancellations, seals, calls, clientCalls)
	}
}
