package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/llm"
)

type terminalErrorMatrixCase struct {
	status cognitionpolicy.CallResultStatus
	code   cognitionpolicy.CallFailureCode
	wants  []error
}

func TestPostgresEveryRegisteredPolicyFailureReplaysExactSentinels(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	cases := terminalErrorMatrixCases()
	if len(cases) != 10 {
		t.Fatalf("registered terminal error matrix has %d entries, want 10", len(cases))
	}
	seen := map[string]bool{}
	for _, test := range cases {
		test := test
		key := string(test.status) + "/" + string(test.code)
		if seen[key] {
			t.Fatalf("duplicate registered terminal error mapping %q", key)
		}
		seen[key] = true
		t.Run(key, func(t *testing.T) {
			fixture := startTaskGenerationRetirementFixtureIn(t, repository, pool, ctx,
				"terminal-error-matrix-"+string(test.code))
			client := terminalMatrixClientFor(t, fixture, test.code)
			journal := captureTerminalMatrixResult(t, fixture, client)
			if journal.result.Status != test.status || journal.result.FailureCode != test.code {
				t.Fatalf("terminal result status/code=%s/%s", journal.result.Status, journal.result.FailureCode)
			}
			assertTerminalMatrixSentinels(t, cognitionpolicy.CallResultError(journal.result), test.wants)
			if err := commitDirectPolicyResult(fixture, journal.attempt, journal.result, journal.evidence); err != nil {
				t.Fatalf("persist registered terminal result: %v", err)
			}
			before := terminalMatrixCounts(t, fixture)
			binding := cognitionruntime.Binding{
				Episode: cognition.EpisodeRef{ID: fixture.EpisodeID},
				Attempt: cognitionAttempt(fixture.Authority),
			}
			recovered, err := repository.ReplayTerminalCognitionPolicyOutcome(ctx, binding)
			if !recovered {
				t.Fatal("same-attempt replay did not recover its durable terminal result")
			}
			assertTerminalMatrixSentinels(t, err, test.wants)
			if got := terminalMatrixCounts(t, fixture); got != before {
				t.Fatalf("same-attempt replay created work: before=%+v after=%+v", before, got)
			}

			replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
			binding.Attempt = cognitionAttempt(replacement)
			recovered, err = repository.ReplayTerminalCognitionPolicyOutcome(ctx, binding)
			if !recovered {
				t.Fatal("replacement-attempt replay did not recover its durable terminal result")
			}
			assertTerminalMatrixSentinels(t, err, test.wants)
			if got := terminalMatrixCounts(t, fixture); got != before {
				t.Fatalf("replacement replay created work: before=%+v after=%+v", before, got)
			}
			if client.providerCalls != 1 {
				t.Fatalf("terminal creation/replays made %d provider calls, want exactly the initial call", client.providerCalls)
			}
		})
	}
}

func terminalErrorMatrixCases() []terminalErrorMatrixCase {
	return []terminalErrorMatrixCase{
		{cognitionpolicy.CallResultRejected, cognitionpolicy.CallFailureResponseLimit, []error{cognitionpolicy.ErrResponseLimit}},
		{cognitionpolicy.CallResultRejected, cognitionpolicy.CallFailureInvalidDecision, []error{cognitionpolicy.ErrInvalidDecision}},
		{cognitionpolicy.CallResultRejected, cognitionpolicy.CallFailureAuthorityDenied, []error{cognitionpolicy.ErrInvalidDecision, cognition.ErrAuthorityDenied}},
		{cognitionpolicy.CallResultRejected, cognitionpolicy.CallFailureProviderUsage, []error{cognitionpolicy.ErrProviderUsage}},
		{cognitionpolicy.CallResultRejected, cognitionpolicy.CallFailureProviderUsageLimit, []error{cognitionpolicy.ErrProviderUsageLimit}},
		{cognitionpolicy.CallResultFailed, cognitionpolicy.CallFailureGeneration, []error{cognitionpolicy.ErrGeneration}},
		{cognitionpolicy.CallResultFailed, cognitionpolicy.CallFailureProviderIdentity, []error{cognitionpolicy.ErrProviderIdentity}},
		{cognitionpolicy.CallResultFailed, cognitionpolicy.CallFailureProviderRequest, []error{cognitionpolicy.ErrGeneration, cognitionpolicy.ErrInvalidEvidence}},
		{cognitionpolicy.CallResultFailed, cognitionpolicy.CallFailurePolicyAuthority, []error{cognitionpolicy.ErrInvalidEvidence}},
		{cognitionpolicy.CallResultFailed, cognitionpolicy.CallFailureProviderEvidence, []error{cognitionpolicy.ErrInvalidEvidence}},
	}
}

type terminalMatrixClient struct {
	cognitionGuardPolicyClient
	providerCalls  int
	mutateContract bool
	transform      func(llm.PreparedModel, llm.PreparedGeneration) (llm.PreparedGeneration, error)
}

func (client *terminalMatrixClient) GeneratePreparedExact(
	ctx context.Context, prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	client.providerCalls++
	generation, err := client.cognitionGuardPolicyClient.GeneratePreparedExact(ctx, prepared)
	if err != nil {
		return generation, err
	}
	if client.mutateContract {
		prepared.ResponseSchema["type"] = "array"
	}
	if client.transform != nil {
		return client.transform(prepared, generation)
	}
	return generation, nil
}

func terminalMatrixClientFor(
	t *testing.T, fixture taskGenerationRetirementFixture, code cognitionpolicy.CallFailureCode,
) *terminalMatrixClient {
	t.Helper()
	client := &terminalMatrixClient{cognitionGuardPolicyClient: cognitionGuardPolicyClient{response: "not-json"}}
	switch code {
	case cognitionpolicy.CallFailureInvalidDecision:
	case cognitionpolicy.CallFailureAuthorityDenied:
		client.response = fmt.Sprintf(`{"obligation_id":%q,"action":{"kind":"observe","arguments":[]},"evidence_refs":[],"expected_effect":"Inspect.","complete":true}`, fixture.Start.Root.ID)
	case cognitionpolicy.CallFailureResponseLimit:
		client.transform = func(prepared llm.PreparedModel, generation llm.PreparedGeneration) (llm.PreparedGeneration, error) {
			generation.ProviderDoneReason = "length"
			generation.Usage.EvalCount = prepared.MaxOutputTokens
			return refreshTerminalMatrixCapture(generation), nil
		}
	case cognitionpolicy.CallFailureProviderUsage:
		client.transform = func(_ llm.PreparedModel, generation llm.PreparedGeneration) (llm.PreparedGeneration, error) {
			generation.ProviderDoneReason = "length"
			generation.Usage.EvalCount = 1
			return refreshTerminalMatrixCapture(generation), nil
		}
	case cognitionpolicy.CallFailureProviderUsageLimit:
		client.transform = func(prepared llm.PreparedModel, generation llm.PreparedGeneration) (llm.PreparedGeneration, error) {
			generation.Usage.PromptEvalCount = prepared.ContextTokens
			return refreshTerminalMatrixCapture(generation), nil
		}
	case cognitionpolicy.CallFailureGeneration:
		client.transform = func(_ llm.PreparedModel, generation llm.PreparedGeneration) (llm.PreparedGeneration, error) {
			generation.Content = ""
			generation.ProviderHTTPStatus = 0
			generation.ProviderResponseDisposition = llm.ProviderResponseTransportError
			generation.ProviderResponseComplete = false
			generation.ProviderContentEncoding = llm.ProviderContentEncodingEvidence{}
			generation.ProviderResponseBytesKnown = false
			generation.ProviderResponseSHA256 = ""
			generation.ProviderResponseBytes = 0
			generation.ProviderResponseCaptureSHA256 = ""
			generation.ProviderResponseCapturedBytes = 0
			generation.ProviderResponseCapture = nil
			generation.ProviderResponseModel = ""
			generation.ProviderDonePresent = false
			generation.ProviderDone = false
			generation.ProviderDoneReason = ""
			generation.UsagePresent = false
			generation.Usage = llm.ProviderGenerationUsage{}
			return generation, errors.New("injected exact provider transport failure")
		}
	case cognitionpolicy.CallFailureProviderIdentity:
		client.transform = func(_ llm.PreparedModel, _ llm.PreparedGeneration) (llm.PreparedGeneration, error) {
			return llm.PreparedGeneration{
				Schema:                     llm.PreparedGenerationSchemaV1,
				ProviderRequestDisposition: llm.ProviderRequestNotDispatched,
				ProviderIdentityEvidence: cognitionProviderFailureEvidence(
					t, fixture.Start.BrainBootstrap.AttestedBrain, llm.ProviderIdentityTokenizer,
				),
			}, errors.New("injected provider identity failure")
		}
	case cognitionpolicy.CallFailureProviderRequest:
		client.transform = func(_ llm.PreparedModel, generation llm.PreparedGeneration) (llm.PreparedGeneration, error) {
			generation.ProviderRequestSHA256 = cognitionGuardSHA([]byte("changed-request"))
			return generation, nil
		}
	case cognitionpolicy.CallFailurePolicyAuthority:
		client.response = `{}`
		client.mutateContract = true
	case cognitionpolicy.CallFailureProviderEvidence:
		client.transform = func(_ llm.PreparedModel, _ llm.PreparedGeneration) (llm.PreparedGeneration, error) {
			return llm.PreparedGeneration{
				Schema:                     llm.PreparedGenerationSchemaV1,
				ProviderRequestDisposition: llm.ProviderRequestNotDispatched,
			}, errors.New("injected contradictory pre-dispatch provider evidence")
		}
	default:
		t.Fatalf("unregistered terminal failure code %q", code)
	}
	return client
}

func refreshTerminalMatrixCapture(generation llm.PreparedGeneration) llm.PreparedGeneration {
	var document map[string]any
	if err := json.Unmarshal(generation.ProviderResponseCapture, &document); err != nil {
		panic(err)
	}
	document["done_reason"] = generation.ProviderDoneReason
	document["prompt_eval_count"] = generation.Usage.PromptEvalCount
	document["eval_count"] = generation.Usage.EvalCount
	raw, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}
	generation.ProviderResponseCapture = raw
	generation.ProviderResponseSHA256 = cognitionGuardSHA(raw)
	generation.ProviderResponseBytes = int64(len(raw))
	generation.ProviderResponseCaptureSHA256 = generation.ProviderResponseSHA256
	generation.ProviderResponseCapturedBytes = len(raw)
	return generation
}

func captureTerminalMatrixResult(
	t *testing.T, fixture taskGenerationRetirementFixture, client *terminalMatrixClient,
) captureStartedPolicyJournal {
	t.Helper()
	prepared, err := fixture.Repository.PrepareCognitionRuntimeSnapshot(fixture.Context,
		CognitionRuntimeSnapshotCommand{Authority: fixture.Authority, EpisodeID: fixture.EpisodeID})
	if err != nil {
		t.Fatal(err)
	}
	journal := captureStartedPolicyJournal{repository: fixture.Repository}
	policy, err := cognitionpolicy.New(client, cognitionTestBrain(), cognitionGuardActivationAuthority(t, fixture),
		cognitionGuardProjectionLoader{repository: fixture.Repository}, &journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(fixture.Context, prepared.Prepared.Snapshot); err == nil {
		t.Fatal("terminal matrix call unexpectedly succeeded")
	}
	if journal.result.Status == "" || journal.result.Validate(journal.attempt) != nil ||
		journal.evidence.ValidateFor(journal.attempt, journal.result) != nil {
		t.Fatalf("captured invalid terminal result/evidence: %+v / %+v", journal.result, journal.evidence)
	}
	return journal
}

type terminalMatrixWorkCounts struct{ snapshots, calls, actions, provider int }

func terminalMatrixCounts(t *testing.T, fixture taskGenerationRetirementFixture) terminalMatrixWorkCounts {
	t.Helper()
	var value terminalMatrixWorkCounts
	if err := fixture.Pool.QueryRow(fixture.Context, `SELECT
	 (SELECT COUNT(*) FROM cognition_runtime_snapshots WHERE episode_id=$1),
	 (SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1),
	 (SELECT COUNT(*) FROM cognition_actions WHERE episode_id=$1),
	 (SELECT COUNT(*) FROM cognition_provider_identity_evidence)+
	 (SELECT COUNT(*) FROM cognition_provider_identity_evidence_operations)+
	 (SELECT COUNT(*) FROM cognition_policy_call_provider_identity_evidence)+
	 (SELECT COUNT(*) FROM cognition_policy_response_evidence)+
	 (SELECT COUNT(*) FROM cognition_policy_provider_response_captures)+
	 (SELECT COUNT(*) FROM cognition_policy_provider_generation_evidence)
	`, fixture.EpisodeID).Scan(&value.snapshots, &value.calls, &value.actions, &value.provider); err != nil {
		t.Fatal(err)
	}
	return value
}

func assertTerminalMatrixSentinels(t *testing.T, got error, wants []error) {
	t.Helper()
	registered := []error{cognitionpolicy.ErrResponseLimit, cognitionpolicy.ErrInvalidDecision,
		cognition.ErrAuthorityDenied, cognitionpolicy.ErrProviderUsage, cognitionpolicy.ErrProviderUsageLimit,
		cognitionpolicy.ErrGeneration, cognitionpolicy.ErrProviderIdentity, cognitionpolicy.ErrInvalidEvidence}
	for _, sentinel := range registered {
		want := false
		for _, expected := range wants {
			want = want || expected == sentinel
		}
		if errors.Is(got, sentinel) != want {
			t.Fatalf("error %v maps errors.Is(%v)=%v, want %v", got, sentinel, errors.Is(got, sentinel), want)
		}
	}
}
