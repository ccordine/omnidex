package cognitiongauntlet

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/cognitionstore"
	"github.com/gryph/omnidex/internal/labyrinth"
	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresPublicFullCognitionSealsRegisteredPolicyFailure(t *testing.T) {
	ctx, pool, repository, hostStore := openFullCognitionDatabase(t)
	fixture, bundle, request := publicFailureFixture(t, ctx, pool, repository, hostStore)
	client := &terminalPolicyClient{
		witnessPolicyClient: &witnessPolicyClient{model: bundle.Authority.RatGeneration.Fixed.Brain.Model},
		failure:             errors.New("registered provider generation failure"),
	}
	request.Client = client
	result, err := RunPublicFullCognition(ctx, bundle, request)
	if err != nil {
		t.Fatal(err)
	}
	assertPublicCanceled(t, pool, result, 1, client.calls())
	if result.Episode.Manifest.Outcome.GoalSatisfied {
		t.Fatal("registered public policy failure was sealed as successful")
	}
	_ = fixture
}

func TestPostgresPublicFullCognitionLeavesProviderDriftLoudAndUnsealed(t *testing.T) {
	ctx, pool, repository, hostStore := openFullCognitionDatabase(t)
	_, bundle, request := publicFailureFixture(t, ctx, pool, repository, hostStore)
	client := &driftingPolicyClient{witnessPolicyClient: &witnessPolicyClient{
		model: bundle.Authority.RatGeneration.Fixed.Brain.Model,
	}}
	request.Client = client
	_, err := RunPublicFullCognition(ctx, bundle, request)
	if !errors.Is(err, cognitionpolicy.ErrProviderIdentity) {
		t.Fatalf("error=%v, want provider identity drift", err)
	}
	assertPublicFailureRemainsUnsealed(t, pool, bundle, request.EpisodeSealPath)
}

func publicFailureFixture(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	repository *queue.Repository,
	hostStore *labyrinthhost.Store,
) (MicrogauntletCase, PublicInferenceBundle, PublicFullCognitionRunRequest) {
	t.Helper()
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[0])
	if err != nil {
		t.Fatal(err)
	}
	claim := claimFailureStep(t, repository, fixture.Spec().Budget.WorkingSetBytes)
	generation := mustRatGeneration(t)
	authority, err := fixture.PairedAuthority(
		SurfaceSymbolic, generation, 1, transferTestFingerprint(),
	)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := NewPublicInferenceBundle(fixture, authority)
	if err != nil {
		t.Fatal(err)
	}
	episode, err := PublicVariantEpisodeRef(bundle.Authority)
	if err != nil {
		t.Fatal(err)
	}
	store, err := cognitionstore.New(repository, cognitionstate.NewNoFactAcceptanceAuthority())
	if err != nil {
		t.Fatal(err)
	}
	scenario := fixture.generated.ExecutionScenario()
	resolver := func(_ context.Context, ref cognition.ScenarioRef) (labyrinth.Scenario, error) {
		if ref != scenario.Ref() {
			return labyrinth.Scenario{}, cognition.ErrInvalidScenario
		}
		return scenario, nil
	}
	surfaceVersion, err := SurfaceSymbolic.Version()
	if err != nil {
		t.Fatal(err)
	}
	environment, err := labyrinthhost.NewSurfaceEnvironment(
		hostStore, episode, resolver, store.AuthorizeAttempt,
		transactionAttemptAuthorizer(repository), surfaceVersion,
	)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, bundle, PublicFullCognitionRunRequest{
		Attempt: claim, Pool: pool, Environment: environment, Completion: environment,
		EpisodeSealPath:     filepath.Join(t.TempDir(), "episode.json"),
		LedgerSchemaVersion: "task-ledger.v1", WorkingSetPolicyVersion: "working-set.v1",
		ProjectionPolicyVersion: "context-projection.v1",
	}
}

type driftingPolicyClient struct {
	*witnessPolicyClient
	mu    sync.Mutex
	calls int
}

func (client *driftingPolicyClient) AttestProviderIdentity(
	ctx context.Context,
	expected llm.ProviderIdentityExpectation,
) (llm.ProviderIdentityAttestation, error) {
	client.mu.Lock()
	client.calls++
	call := client.calls
	client.mu.Unlock()
	if call > 1 {
		return llm.ProviderIdentityAttestation{}, errors.New("provider identity changed after episode bootstrap")
	}
	return client.witnessPolicyClient.AttestProviderIdentity(ctx, expected)
}

func assertPublicFailureRemainsUnsealed(
	t *testing.T,
	pool *pgxpool.Pool,
	bundle PublicInferenceBundle,
	episodePath string,
) {
	t.Helper()
	episode, err := PublicVariantEpisodeRef(bundle.Authority)
	if err != nil {
		t.Fatal(err)
	}
	var status string
	var cancellations, seals int
	if err := pool.QueryRow(t.Context(), `
		SELECT episodes.status,
		       (SELECT COUNT(*) FROM cognition_episode_cancellations WHERE episode_id=episodes.episode_id),
		       (SELECT COUNT(*) FROM cognition_terminal_seals WHERE episode_id=episodes.episode_id)
		FROM cognition_episodes episodes WHERE episodes.episode_id=$1
	`, episode.ID).Scan(&status, &cancellations, &seals); err != nil {
		t.Fatal(err)
	}
	if status != string(queue.CognitionEpisodeActive) || cancellations != 0 || seals != 0 {
		t.Fatalf("status/cancellations/seals=%s/%d/%d", status, cancellations, seals)
	}
	if _, err := os.Stat(episodePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("loud public failure wrote an episode seal: %v", err)
	}
}

func assertPublicCanceled(
	t *testing.T,
	pool *pgxpool.Pool,
	result PublicFullCognitionRunResult,
	wantCalls int,
	clientCalls int,
) {
	t.Helper()
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if result.Episode.Manifest.Outcome.GoalSatisfied ||
		result.Episode.Manifest.Outcome.FailureCode == "" {
		t.Fatalf("canceled public result=%+v", result)
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

var _ llm.Client = (*driftingPolicyClient)(nil)
