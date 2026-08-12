package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
)

func TestVerifyCognitionProviderActivationFailureTerminalAuthorityRejectsEveryAssociationChange(t *testing.T) {
	bootstrap, failure, record := cognitionProviderFailureTerminalFixture(t)
	if err := VerifyCognitionProviderProcessFailureTraceIdentity(
		record.ID, bootstrap, failure,
	); err != nil {
		t.Fatal(err)
	}
	cancellation, err := cognitionruntime.NewProviderActivationCancellationEvidence(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := providerProcessObservationAuthority(failure.Receipt.Actor)
	if err != nil {
		t.Fatal(err)
	}
	seal := CognitionTerminalSeal{
		EpisodeID: failure.Receipt.EpisodeID, Outcome: CognitionEpisodeCanceled,
		AuthorityKind: cognitionTerminalAuthorityWorker, SealedBy: authority,
	}
	verify := func(
		record CognitionSealedTraceRecord,
		cancellation cognitionruntime.CancellationEvidence,
		seal CognitionTerminalSeal,
	) error {
		return VerifyCognitionProviderActivationFailureTerminalAuthority(
			record, bootstrap, failure, cancellation, seal,
		)
	}
	if err := verify(record, cancellation, seal); err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*CognitionSealedTraceRecord, *cognitionruntime.CancellationEvidence, *CognitionTerminalSeal){
		"kind": func(record *CognitionSealedTraceRecord, _ *cognitionruntime.CancellationEvidence, _ *CognitionTerminalSeal) {
			record.Kind = "provider_process_observation"
		},
		"tuple": func(record *CognitionSealedTraceRecord, _ *cognitionruntime.CancellationEvidence, _ *CognitionTerminalSeal) {
			record.Phase++
		},
		"payload": func(record *CognitionSealedTraceRecord, _ *cognitionruntime.CancellationEvidence, _ *CognitionTerminalSeal) {
			record.Payload = append([]byte(nil), record.Payload...)
			record.Payload[len(record.Payload)-1] = ' '
		},
		"self-consistent record identity": func(record *CognitionSealedTraceRecord, cancellation *cognitionruntime.CancellationEvidence, _ *CognitionTerminalSeal) {
			record.ID = "cognition_provider_failure_" + strings.Repeat("f", 64)
			changed, err := cognitionruntime.NewProviderActivationCancellationEvidence(record.ID)
			if err != nil {
				t.Fatal(err)
			}
			*cancellation = changed
		},
		"cancellation": func(_ *CognitionSealedTraceRecord, cancellation *cognitionruntime.CancellationEvidence, _ *CognitionTerminalSeal) {
			cancellation.PublicMessage = "changed"
		},
		"outcome": func(_ *CognitionSealedTraceRecord, _ *cognitionruntime.CancellationEvidence, seal *CognitionTerminalSeal) {
			seal.Outcome = CognitionEpisodeFailed
		},
		"authority": func(_ *CognitionSealedTraceRecord, _ *cognitionruntime.CancellationEvidence, seal *CognitionTerminalSeal) {
			seal.AuthorityKind = cognitionTerminalAuthorityLifecycle
		},
		"episode": func(_ *CognitionSealedTraceRecord, _ *cognitionruntime.CancellationEvidence, seal *CognitionTerminalSeal) {
			seal.EpisodeID = "episode-changed"
		},
		"actor": func(_ *CognitionSealedTraceRecord, _ *cognitionruntime.CancellationEvidence, seal *CognitionTerminalSeal) {
			seal.SealedBy.Attempt++
		},
		"lifecycle": func(_ *CognitionSealedTraceRecord, _ *cognitionruntime.CancellationEvidence, seal *CognitionTerminalSeal) {
			seal.LifecycleOperationID = "lifecycle-operation"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changedRecord, changedCancellation, changedSeal := record, cancellation, seal
			mutate(&changedRecord, &changedCancellation, &changedSeal)
			if verify(changedRecord, changedCancellation, changedSeal) == nil {
				t.Fatal("changed provider failure terminal association was accepted")
			}
		})
	}
}

func cognitionProviderFailureTerminalFixture(t *testing.T) (
	cognitionpolicy.BrainBootstrap,
	cognitionpolicy.ProviderProcessFailure,
	CognitionSealedTraceRecord,
) {
	t.Helper()
	bootstrap := cognitionTestBrainBootstrap()
	evidence := cognitionProviderFailureEvidence(
		t, bootstrap.AttestedBrain, llm.ProviderIdentityPreload,
	)
	episode := cognition.EpisodeRef{
		ID: cognition.EpisodeID("episode-" + strings.Repeat("a", 64)),
	}
	actor := cognition.AttemptRef{
		JobID: 1, Generation: 1, StepID: 1, Attempt: 1,
		WorkerID: "worker-provider-failure-terminal",
	}
	outcome, observeErr := cognitionpolicy.ObserveProviderProcess(
		t.Context(), cognitionFailurePolicyClient{evidence: evidence},
		bootstrap.AttestedBrain, episode, actor,
		cognitionpolicy.ProviderProcessEpisodeInvocation,
	)
	if observeErr == nil || outcome.Failure == nil {
		t.Fatalf("provider failure outcome=%+v error=%v", outcome, observeErr)
	}
	receiptJSON, err := exactjson.Canonical(outcome.Failure.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareProviderFailureBootstrap(&bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := providerProcessObservationAuthority(outcome.Failure.Receipt.Actor)
	if err != nil {
		t.Fatal(err)
	}
	bound, _, err := newCognitionProviderFailureAuthority(
		cognitionProviderFailureProcess, outcome.Failure.Receipt.ID,
		outcome.Failure.Receipt.EpisodeID, authority, evidence.Ref.ID,
		cognitionPayloadSHA(receiptJSON), prepared,
	)
	if err != nil {
		t.Fatal(err)
	}
	return bootstrap, *outcome.Failure, CognitionSealedTraceRecord{
		Kind: "provider_activation_failure", Phase: 4, Sequence: 1,
		ID: bound.RecordID, SHA256: cognitionPayloadSHA(receiptJSON), Payload: receiptJSON,
	}
}
