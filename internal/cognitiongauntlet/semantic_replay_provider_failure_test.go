package cognitiongauntlet

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticReplayProviderFailureBindsExactTraceIdentityAndTuple(t *testing.T) {
	state, bootstrapRecord, failureRecord := semanticReplayProviderFailureFixture(t)
	if _, err := state.mapProviderBrainBootstrap(
		bootstrapRecord, semanticReplaySourceForRecord(t, 1, bootstrapRecord),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := state.mapProviderActivationFailure(
		failureRecord, semanticReplaySourceForRecord(t, 2, failureRecord),
	); err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func(*queue.CognitionSealedTraceRecord, *queue.CognitionSealedTraceRecord){
		"failure phase": func(_ *queue.CognitionSealedTraceRecord, failure *queue.CognitionSealedTraceRecord) {
			failure.Phase++
		},
		"failure sequence": func(_ *queue.CognitionSealedTraceRecord, failure *queue.CognitionSealedTraceRecord) {
			failure.Sequence++
		},
		"bootstrap phase": func(bootstrap *queue.CognitionSealedTraceRecord, _ *queue.CognitionSealedTraceRecord) {
			bootstrap.Phase++
		},
		"self-consistent renamed record": func(bootstrap *queue.CognitionSealedTraceRecord, failure *queue.CognitionSealedTraceRecord) {
			failure.ID = "cognition_provider_failure_" + strings.Repeat("f", 64)
			var value queue.CognitionBrainBootstrapTrace
			if err := decodeProductionPayload(bootstrap.Payload, &value, "test bootstrap"); err != nil {
				t.Fatal(err)
			}
			value.SourceID = failure.ID
			bootstrap.ID = failure.ID
			bootstrap.Payload = semanticReplayJSON(t, value)
			bootstrap.SHA256 = digestExactBytes(bootstrap.Payload)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changedState, changedBootstrap, changedFailure := semanticReplayProviderFailureFixture(t)
			mutate(&changedBootstrap, &changedFailure)
			if _, err := changedState.mapProviderBrainBootstrap(
				changedBootstrap, semanticReplaySourceForRecord(t, 1, changedBootstrap),
			); err != nil {
				return
			}
			if _, err := changedState.mapProviderActivationFailure(
				changedFailure, semanticReplaySourceForRecord(t, 2, changedFailure),
			); err == nil {
				t.Fatal("semantic mapper accepted changed provider failure identity or tuple")
			}
		})
	}
}

func TestSemanticReplayProviderFailureRequiresExactCancellationAndWorkerSeal(t *testing.T) {
	state, bootstrapRecord, failureRecord := semanticReplayProviderFailureFixture(t)
	if _, err := state.mapProviderBrainBootstrap(
		bootstrapRecord, semanticReplaySourceForRecord(t, 1, bootstrapRecord),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := state.mapProviderActivationFailure(
		failureRecord, semanticReplaySourceForRecord(t, 2, failureRecord),
	); err != nil {
		t.Fatal(err)
	}
	failure := state.activationFailures[failureRecord.ID]
	cancellation, err := cognitionruntime.NewProviderActivationCancellationEvidence(failureRecord.ID)
	if err != nil {
		t.Fatal(err)
	}
	actor := failure.failure.Receipt.Actor
	state.cancellation = &cancellation
	state.trace.Header.Seal = queue.CognitionTerminalSeal{
		EpisodeID: failure.failure.Receipt.EpisodeID, Outcome: queue.CognitionEpisodeCanceled,
		AuthorityKind: "worker",
		SealedBy: model.StepAttemptAuthority{
			JobID: actor.JobID, Generation: actor.Generation, StepID: actor.StepID,
			Attempt: int64(actor.Attempt), WorkerID: actor.WorkerID,
		},
	}
	if err := state.finishProviderActivationFailure(); err != nil {
		t.Fatal(err)
	}

	changed := *state
	changed.trace.Header.Seal.SealedBy.Attempt++
	if err := changed.finishProviderActivationFailure(); err == nil {
		t.Fatal("provider activation failure with changed terminal actor was accepted")
	}
	changed = *state
	changed.cancellation = nil
	if err := changed.finishProviderActivationFailure(); err == nil {
		t.Fatal("provider activation failure without its cancellation was accepted")
	}
	changed = *state
	changed.activationFailures = map[string]semanticActivationFailure{}
	if err := changed.finishProviderActivationFailure(); err == nil {
		t.Fatal("provider activation cancellation without its failure was accepted")
	}
	other, err := cognitionruntime.NewCancellationEvidence(
		cognitionruntime.CancellationPolicyFailure, "Policy failed.",
		fmt.Errorf("policy failed"),
	)
	if err != nil {
		t.Fatal(err)
	}
	changed = *state
	changed.cancellation = &other
	if err := changed.finishProviderActivationFailure(); err == nil {
		t.Fatal("provider activation failure using another cancellation was accepted")
	}
}

func semanticReplayProviderFailureFixture(t *testing.T) (
	*semanticReplayState,
	queue.CognitionSealedTraceRecord,
	queue.CognitionSealedTraceRecord,
) {
	t.Helper()
	frozen, err := mustRatGeneration(t).Fixed.Brain.attestedBrain()
	if err != nil {
		t.Fatal(err)
	}
	bootstrapEvidence, err := witnessProviderIdentityEvidence(frozen.Attestation)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := cognitionpolicy.NewBrainBootstrap(frozen, bootstrapEvidence)
	if err != nil {
		t.Fatal(err)
	}
	episode := cognition.EpisodeID("episode-" + strings.Repeat("a", 64))
	actor := cognition.AttemptRef{
		JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "worker-provider-failure",
	}
	client := &providerIdentityFailureClient{
		witnessPolicyClient: &witnessPolicyClient{model: frozen.Ref.Model}, failAt: 1,
	}
	outcome, observeErr := cognitionpolicy.ObserveProviderProcess(
		t.Context(), client, frozen, cognition.EpisodeRef{ID: episode}, actor,
		cognitionpolicy.ProviderProcessEpisodeInvocation,
	)
	if observeErr == nil || outcome.Failure == nil {
		t.Fatalf("provider failure outcome=%+v error=%v", outcome, observeErr)
	}
	recordID := independentProviderFailureRecordID(t, bootstrap, *outcome.Failure)
	recordedAt := time.Date(2026, 8, 12, 1, 2, 3, 0, time.UTC)
	bootstrapTrace := queue.CognitionBrainBootstrapTrace{
		Schema: queue.CognitionBrainBootstrapTraceSchemaV1,
		Source: queue.CognitionBrainBootstrapActivationFailure, SourceID: recordID,
		EpisodeID: episode, Actor: actor, Brain: frozen,
		Evidence: bootstrapEvidence.Ref, RecordedAt: recordedAt,
	}
	bootstrapRecord := semanticReplayRawRecord(
		queue.CognitionTraceKindProviderBrainBootstrap, 0, 3, 1,
		recordID, semanticReplayJSON(t, bootstrapTrace),
	)
	receiptRaw, err := exactjson.Canonical(outcome.Failure.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	failureRecord := semanticReplayRawRecord(
		"provider_activation_failure", 0, 4, 1, recordID, receiptRaw,
	)
	state := newSemanticReplayState(
		productionTrace{Header: queue.CognitionSealedTracePage{
			EpisodeID: episode, EpisodeStartedAt: recordedAt.Add(-time.Minute),
			SealedAt: recordedAt.Add(time.Minute),
		}}, nil, nil,
		frozen, cognition.GoalExpression{}, cognition.CompletionAuthority{}, cognition.ActionCatalog{},
		cognition.RuntimeBudget{},
		semanticReplaySupplement{identity: map[string]llm.ProviderIdentityEvidence{
			bootstrapEvidence.Ref.ID:                bootstrapEvidence,
			outcome.Failure.IdentityEvidence.Ref.ID: outcome.Failure.IdentityEvidence,
		}},
	)
	return state, bootstrapRecord, failureRecord
}

func semanticReplaySourceForRecord(
	t *testing.T,
	ordinal uint64,
	record queue.CognitionSealedTraceRecord,
) cognitionreplay.SourceRecord {
	t.Helper()
	return semanticReplayGraphSource(t, ordinal, record)
}

func independentProviderFailureRecordID(
	t *testing.T,
	bootstrap cognitionpolicy.BrainBootstrap,
	failure cognitionpolicy.ProviderProcessFailure,
) string {
	t.Helper()
	receiptRaw, err := exactjson.Canonical(failure.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	brainRaw, err := exactjson.Canonical(bootstrap.AttestedBrain)
	if err != nil {
		t.Fatal(err)
	}
	authority := struct {
		Schema               string               `json:"schema"`
		RecordID             string               `json:"record_id"`
		FailureKind          string               `json:"failure_kind"`
		FailureID            string               `json:"failure_id"`
		EpisodeID            cognition.EpisodeID  `json:"episode_id"`
		Actor                cognition.AttemptRef `json:"actor"`
		EvidenceID           string               `json:"evidence_id"`
		ReceiptSHA256        string               `json:"receipt_sha256"`
		BootstrapEvidenceID  string               `json:"bootstrap_evidence_id"`
		BootstrapBrainSHA256 string               `json:"bootstrap_brain_sha256"`
	}{
		Schema:      "omnidex.cognition-provider-failure-authority.v1",
		FailureKind: "provider_process", FailureID: failure.Receipt.ID,
		EpisodeID: failure.Receipt.EpisodeID, Actor: failure.Receipt.Actor,
		EvidenceID:           failure.IdentityEvidence.Ref.ID,
		ReceiptSHA256:        digestExactBytes(receiptRaw),
		BootstrapEvidenceID:  bootstrap.BootstrapEvidence.Ref.ID,
		BootstrapBrainSHA256: digestExactBytes(brainRaw),
	}
	raw, err := exactjson.Canonical(authority)
	if err != nil {
		t.Fatal(err)
	}
	return "cognition_provider_failure_" + digestExactBytes(raw)
}
