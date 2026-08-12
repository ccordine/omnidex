package queue

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/ollama"
)

func TestPostgresOllamaIdentityFailuresTerminalizeWithoutGeneration(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	tests := map[string]struct {
		status      int
		body        string
		closeServer bool
		want        string
	}{
		"transport":    {closeServer: true, want: "transport_error"},
		"http":         {status: http.StatusServiceUnavailable, body: `{}`, want: "http_error"},
		"invalid_body": {status: http.StatusOK, body: `{`, want: "invalid_json"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := startTaskGenerationRetirementFixtureIn(
				t, repository, pool, ctx, "ollama-identity-failure-"+name,
			)
			server, generateRequests := newFailingOllamaIdentityServer(t, test.status, test.body)
			if test.closeServer {
				server.Close()
			}
			client := ollama.New(
				server.URL, fixture.Start.BrainBootstrap.AttestedBrain.Ref.Model, "",
				2*time.Second,
				fixture.Start.BrainBootstrap.AttestedBrain.Ref.NativeContextLimit,
			)
			if err := reserveTerminalCognitionPolicyCall(t, fixture, client); !errors.Is(err, cognitionpolicy.ErrProviderIdentity) {
				t.Fatalf("fresh Ollama identity failure=%v", err)
			}
			assertDurableOllamaIdentityFailure(t, fixture, test.want, generateRequests)

			binding := cognitionruntime.Binding{
				Episode: cognition.EpisodeRef{ID: fixture.EpisodeID},
				Attempt: cognitionAttempt(fixture.Authority),
			}
			recovered, err := repository.ReplayTerminalCognitionPolicyOutcome(ctx, binding)
			if !recovered || !errors.Is(err, cognitionpolicy.ErrProviderIdentity) {
				t.Fatalf("Ollama identity replay recovered=%v error=%v", recovered, err)
			}
			assertDurableOllamaIdentityFailure(t, fixture, test.want, generateRequests)
		})
	}
}

func TestPostgresOllamaIdentityFailureCannotBeRelabeledAsInvalidEvidence(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "ollama-identity-failure-relabel",
	)
	server, _ := newFailingOllamaIdentityServer(t, http.StatusServiceUnavailable, `{}`)
	client := ollama.New(
		server.URL, fixture.Start.BrainBootstrap.AttestedBrain.Ref.Model, "", 2*time.Second,
		fixture.Start.BrainBootstrap.AttestedBrain.Ref.NativeContextLimit,
	)
	journal := captureOllamaIdentityFailure(t, fixture, client)
	forged := journal.result
	forged.FailureCode = cognitionpolicy.CallFailureProviderEvidence
	if err := commitDirectPolicyResult(
		fixture, journal.attempt, forged, journal.evidence,
	); err == nil {
		t.Fatal("direct SQL relabeled exact provider identity failure evidence")
	}
	var status string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM cognition_policy_calls WHERE call_id=$1`, journal.attempt.ID,
	).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "started" {
		t.Fatalf("rejected relabel changed call status to %q", status)
	}
}

func newFailingOllamaIdentityServer(
	t *testing.T,
	status int,
	body string,
) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	generateRequests := &atomic.Int64{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/api/generate" {
			generateRequests.Add(1)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(status)
		_, _ = writer.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, generateRequests
}

func assertDurableOllamaIdentityFailure(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	wantDisposition string,
	generateRequests *atomic.Int64,
) {
	t.Helper()
	var status, failureCode, requestDisposition, firstDisposition string
	var calls, operations, captures, generations, actions int
	if err := fixture.Pool.QueryRow(fixture.Context, `SELECT
		calls.status,calls.result_json::jsonb->>'failure_code',
		calls.result_json::jsonb->>'provider_request_disposition',operations.disposition,
		(SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_provider_identity_evidence_operations raw
		 WHERE raw.evidence_id=association.evidence_id),
		(SELECT COUNT(*) FROM cognition_policy_provider_response_captures WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_policy_provider_generation_evidence WHERE episode_id=$1),
		(SELECT COUNT(*) FROM cognition_actions WHERE episode_id=$1)
	FROM cognition_policy_calls calls
	JOIN cognition_policy_call_provider_identity_evidence association USING (call_id)
	JOIN cognition_provider_identity_evidence_operations operations
	  ON operations.evidence_id=association.evidence_id AND operations.operation_index=0
	WHERE calls.episode_id=$1`, fixture.EpisodeID).Scan(
		&status, &failureCode, &requestDisposition, &firstDisposition,
		&calls, &operations, &captures, &generations, &actions,
	); err != nil {
		t.Fatal(err)
	}
	if status != string(cognitionpolicy.CallResultFailed) ||
		failureCode != string(cognitionpolicy.CallFailureProviderIdentity) ||
		requestDisposition != "not_dispatched" || firstDisposition != wantDisposition ||
		calls != 1 || operations != 5 || captures != 0 || generations != 0 || actions != 0 ||
		generateRequests.Load() != 0 {
		t.Fatalf("durable Ollama identity failure=%s/%s/%s/%s rows=%d/%d/%d/%d/%d generation_requests=%d",
			status, failureCode, requestDisposition, firstDisposition,
			calls, operations, captures, generations, actions, generateRequests.Load())
	}
}

func captureOllamaIdentityFailure(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	client *ollama.Client,
) captureStartedPolicyJournal {
	t.Helper()
	prepared, err := fixture.Repository.PrepareCognitionRuntimeSnapshot(
		fixture.Context,
		CognitionRuntimeSnapshotCommand{Authority: fixture.Authority, EpisodeID: fixture.EpisodeID},
	)
	if err != nil {
		t.Fatal(err)
	}
	journal := captureStartedPolicyJournal{repository: fixture.Repository}
	policy, err := cognitionpolicy.New(
		client, cognitionTestBrain(), cognitionGuardActivationAuthority(t, fixture),
		cognitionGuardProjectionLoader{repository: fixture.Repository}, &journal,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(fixture.Context, prepared.Prepared.Snapshot); !errors.Is(err, cognitionpolicy.ErrProviderIdentity) {
		t.Fatalf("captured Ollama identity error=%v", err)
	}
	if journal.result.FailureCode != cognitionpolicy.CallFailureProviderIdentity ||
		journal.evidence.ProviderIdentity.Validate() != nil {
		t.Fatalf("captured Ollama identity result=%+v", journal.result)
	}
	return journal
}
