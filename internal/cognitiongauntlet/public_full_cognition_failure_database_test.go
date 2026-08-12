package cognitiongauntlet

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/cognitionstore"
	"github.com/gryph/omnidex/internal/labyrinth"
	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresPublicFullCognitionSealsExactPolicyFailureDispositions(t *testing.T) {
	for _, disposition := range []llm.ProviderRequestDisposition{
		llm.ProviderRequestDispatched,
		llm.ProviderRequestNotDispatched,
		llm.ProviderRequestWriteIndeterminate,
	} {
		t.Run(string(disposition), func(t *testing.T) {
			ctx, pool, repository, hostStore := openFullCognitionDatabase(t)
			_, bundle, request := publicFailureFixture(t, ctx, pool, repository, hostStore)
			client := &terminalPolicyClient{
				witnessPolicyClient: &witnessPolicyClient{
					model: bundle.Authority.RatGeneration.Fixed.Brain.Model,
				},
				failure:            errors.New("registered provider generation failure"),
				requestDisposition: disposition,
			}
			request.Client = client
			result, err := RunPublicFullCognition(ctx, bundle, request)
			if err != nil {
				t.Fatal(err)
			}
			assertPublicCanceled(t, pool, result, 1, client.calls())
			resources := result.Episode.Manifest.Resources
			if result.Episode.Manifest.Outcome.GoalSatisfied || resources.PolicyCallsConsumed != 1 {
				t.Fatalf("registered failure outcome/resources=%+v/%+v", result.Episode.Manifest.Outcome, resources)
			}
			wantModelCalls := 0
			wantTraceKind := TracePolicyDisposition
			if disposition == llm.ProviderRequestDispatched {
				wantModelCalls = 1
				wantTraceKind = TraceModelCall
			}
			if resources.ModelCalls != wantModelCalls ||
				(resources.ModelCalls == 0 && (resources.ContextBytes != 0 || resources.InputTokens != 0)) {
				t.Fatalf("request disposition %q invented model usage: %+v", disposition, resources)
			}
			assertPolicyTerminalTraceDisposition(t, result.Episode, wantTraceKind, disposition)
		})
	}
}

func assertPolicyTerminalTraceDisposition(
	t *testing.T,
	episode SealedEpisode,
	wantKind TraceKind,
	wantDisposition llm.ProviderRequestDisposition,
) {
	t.Helper()
	for _, entry := range episode.Manifest.Trace {
		if entry.Kind != wantKind {
			continue
		}
		if wantKind == TraceModelCall {
			var call ModelCallTrace
			if err := decodeTracePayload(entry.Payload, &call, "failure model call"); err != nil {
				t.Fatal(err)
			}
			if call.ProviderRequestDisposition != wantDisposition {
				t.Fatalf("model call disposition=%q want %q", call.ProviderRequestDisposition, wantDisposition)
			}
			return
		}
		var disposition PolicyDispositionTrace
		if err := decodeTracePayload(entry.Payload, &disposition, "failure policy disposition"); err != nil {
			t.Fatal(err)
		}
		if disposition.Disposition != PolicyCallResultDisposition ||
			disposition.ProviderRequestDisposition != wantDisposition {
			t.Fatalf("policy disposition=%+v want %q", disposition, wantDisposition)
		}
		return
	}
	t.Fatalf("sealed failure trace lacks %q disposition %q", wantKind, wantDisposition)
}

func publicFailureFixture(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	repository *queue.Repository,
	hostStore *labyrinthhost.Store,
) (MicrogauntletCase, PublicInferenceBundle, PublicFullCognitionRunRequest) {
	t.Helper()
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV2()[0])
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
