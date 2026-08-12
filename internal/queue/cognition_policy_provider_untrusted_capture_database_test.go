package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/llm"
)

type dispatchedMalformedProviderClient struct {
	cognitionGuardPolicyClient
	providerCalls int
}

func (client *dispatchedMalformedProviderClient) GeneratePreparedExact(
	ctx context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	client.providerCalls++
	generation, err := client.cognitionGuardPolicyClient.GeneratePreparedExact(ctx, prepared)
	if err != nil {
		return generation, err
	}
	return dispatchedMalformedGeneration(generation), nil
}

func dispatchedMalformedGeneration(generation llm.PreparedGeneration) llm.PreparedGeneration {
	generation.ProviderDoneReason = "length"
	generation.Usage.EvalCount = 1
	generation = refreshDispatchedMalformedCapture(generation)
	generation.ProviderResponseSHA256 = ""
	return generation
}

func TestPostgresDispatchedMalformedProviderGenerationRetainsOpaqueCapture(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "dispatched-malformed-provider-capture",
	)
	client := &dispatchedMalformedProviderClient{
		cognitionGuardPolicyClient: cognitionGuardPolicyClient{response: "not-json"},
	}
	freshErr := reserveTerminalCognitionPolicyCall(t, fixture, client)
	if !errors.Is(freshErr, cognitionpolicy.ErrInvalidEvidence) {
		t.Fatalf("fresh malformed provider error=%v", freshErr)
	}
	var status, failureCode string
	var captures, generations int
	if err := pool.QueryRow(ctx, `SELECT status,COALESCE(result_json::jsonb->>'failure_code',''),
	 (SELECT COUNT(*) FROM cognition_policy_provider_response_captures WHERE episode_id=$1),
	 (SELECT COUNT(*) FROM cognition_policy_provider_generation_evidence WHERE episode_id=$1)
	 FROM cognition_policy_calls WHERE episode_id=$1`, fixture.EpisodeID).Scan(
		&status, &failureCode, &captures, &generations,
	); err != nil {
		t.Fatal(err)
	}
	if status != string(cognitionpolicy.CallResultFailed) ||
		failureCode != string(cognitionpolicy.CallFailureProviderEvidence) ||
		captures != 1 || generations != 1 || client.providerCalls != 1 {
		t.Fatalf("durable malformed provider=%s/%s captures/generations/calls=%d/%d/%d error=%v",
			status, failureCode, captures, generations, client.providerCalls, freshErr)
	}

	before := terminalMatrixCounts(t, fixture)
	binding := cognitionruntime.Binding{
		Episode: cognition.EpisodeRef{ID: fixture.EpisodeID},
		Attempt: cognitionAttempt(fixture.Authority),
	}
	assertDispatchedMalformedReplay(t, fixture, binding, before)
	replacement := replaceCognitionAttemptForTest(t, pool, fixture.Authority)
	binding.Attempt = cognitionAttempt(replacement)
	assertDispatchedMalformedReplay(t, fixture, binding, before)
	if client.providerCalls != 1 {
		t.Fatalf("durable malformed replay made %d provider calls", client.providerCalls)
	}
}

func TestPostgresOpaqueProviderCaptureRejectsBytesOutsideGenerationEvidence(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, ctx, "opaque-provider-capture-substitution",
	)
	client := &terminalMatrixClient{
		cognitionGuardPolicyClient: cognitionGuardPolicyClient{response: "not-json"},
		transform: func(
			_ llm.PreparedModel, generation llm.PreparedGeneration,
		) (llm.PreparedGeneration, error) {
			return dispatchedMalformedGeneration(generation), nil
		},
	}
	journal := captureTerminalMatrixResult(t, fixture, client)
	forgedResult := journal.result
	forgedEvidence := journal.evidence.Clone()
	changed := append([]byte(nil), forgedEvidence.ProviderResponseCapture.Content...)
	if len(changed) == 0 {
		t.Fatal("malformed provider fixture has no raw capture")
	}
	changed[len(changed)-1] ^= 1
	if bytes.Equal(changed, forgedEvidence.ProviderResponseCapture.Content) {
		t.Fatal("capture substitution did not change raw bytes")
	}
	capture, err := cognitionpolicy.NewProviderResponseCaptureEvidence(journal.attempt.ID, changed)
	if err != nil {
		t.Fatal(err)
	}
	forgedEvidence.ProviderResponseCapture = capture
	forgedResult.ProviderResponseCapture = capture.Ref
	forgedResult.ProviderGenerationEvidence = journal.result.ProviderGenerationEvidence
	if err := forgedResult.Validate(journal.attempt); err != nil {
		t.Fatalf("forged result shape should remain Go-valid before raw association: %v", err)
	}
	if err := commitDirectPolicyResult(
		fixture, journal.attempt, forgedResult, forgedEvidence,
	); err == nil {
		t.Fatal("opaque provider capture not named by generation evidence committed")
	}
	var status string
	if err := pool.QueryRow(ctx,
		`SELECT status FROM cognition_policy_calls WHERE call_id=$1`, journal.attempt.ID,
	).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "started" {
		t.Fatalf("rejected opaque capture substitution changed status to %q", status)
	}
}

func TestPostgresProviderCaptureProjectionFunctionIsImmutableAndTableFree(t *testing.T) {
	_, pool, ctx := policyInputFreshRepository(t)
	var volatility string
	var definition string
	if err := pool.QueryRow(ctx, `SELECT p.provolatile::text,pg_get_functiondef(p.oid)
	 FROM pg_proc p WHERE p.proname='cognition_provider_response_capture_projects_result'
	 AND pg_get_function_identity_arguments(p.oid)='status_code integer, captured bytea, result jsonb'`,
	).Scan(&volatility, &definition); err != nil {
		t.Fatal(err)
	}
	if volatility != "i" || bytes.Contains(
		[]byte(definition), []byte("cognition_policy_provider_generation_evidence"),
	) {
		t.Fatalf("pure capture projector volatility/body=%q/%s", volatility, definition)
	}
}

func assertDispatchedMalformedReplay(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	binding cognitionruntime.Binding,
	want terminalMatrixWorkCounts,
) {
	t.Helper()
	recovered, err := fixture.Repository.ReplayTerminalCognitionPolicyOutcome(
		fixture.Context, binding,
	)
	if !recovered || !errors.Is(err, cognitionpolicy.ErrInvalidEvidence) {
		t.Fatalf("malformed provider replay recovered=%t error=%v", recovered, err)
	}
	if got := terminalMatrixCounts(t, fixture); got != want {
		t.Fatalf("malformed provider replay created work: before=%+v after=%+v", want, got)
	}
}

func refreshDispatchedMalformedCapture(
	generation llm.PreparedGeneration,
) llm.PreparedGeneration {
	var document map[string]any
	if err := json.Unmarshal(generation.ProviderResponseCapture, &document); err != nil {
		panic(err)
	}
	document["done_reason"] = generation.ProviderDoneReason
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
