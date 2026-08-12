package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
)

func TestVerifyCognitionProviderProcessFailureTraceIdentityUsesDurableDerivation(t *testing.T) {
	bootstrap := cognitionTestBrainBootstrap()
	evidence := cognitionProviderFailureEvidence(
		t, bootstrap.AttestedBrain, llm.ProviderIdentityPreload,
	)
	episode := cognition.EpisodeRef{ID: cognition.EpisodeID("episode-" + strings.Repeat("a", 64))}
	actor := cognition.AttemptRef{
		JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "worker-failure-verifier",
	}
	outcome, observeErr := cognitionpolicy.ObserveProviderProcess(
		t.Context(), cognitionFailurePolicyClient{evidence: evidence},
		bootstrap.AttestedBrain, episode, actor,
		cognitionpolicy.ProviderProcessEpisodeInvocation,
	)
	if observeErr == nil || outcome.Failure == nil {
		t.Fatalf("provider process outcome=%+v error=%v", outcome, observeErr)
	}
	receiptJSON, err := exactjson.Canonical(outcome.Failure.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareProviderFailureBootstrap(&bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := providerProcessObservationAuthority(actor)
	if err != nil {
		t.Fatal(err)
	}
	want, _, err := newCognitionProviderFailureAuthority(
		cognitionProviderFailureProcess, outcome.Failure.Receipt.ID, episode.ID,
		authority, evidence.Ref.ID, cognitionPayloadSHA(receiptJSON), prepared,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyCognitionProviderProcessFailureTraceIdentity(
		want.RecordID, bootstrap, *outcome.Failure,
	); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCognitionProviderProcessFailureTraceIdentity(
		"cognition_provider_failure_"+strings.Repeat("f", 64),
		bootstrap, *outcome.Failure,
	); err == nil {
		t.Fatal("provider process failure verifier accepted another trace record ID")
	}
}
