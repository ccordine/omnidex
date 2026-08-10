package cognitionpolicy

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestPolicyStartFailurePreventsInference(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, evidence := policyTestSnapshot(t, projection)
	client := &policyTestClient{response: policyTestResponse(t, snapshot, evidence)}
	journal := &policyTestCallJournal{startErr: errors.New("database unavailable")}
	policy, err := New(client, policyTestAttestedBrain(), newPolicyTestProjectionLoader(projection), journal)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := policy.Decide(context.Background(), snapshot)
	if !errors.Is(err, ErrCallJournal) {
		t.Fatalf("error=%v want ErrCallJournal", err)
	}
	if outcome.ProviderRequestDispatched {
		t.Fatal("call reservation failure reported inference")
	}
	if client.generateCalls != 0 || len(journal.attempts) != 1 || len(journal.results) != 0 {
		t.Fatalf("generate=%d attempts=%d results=%d", client.generateCalls, len(journal.attempts), len(journal.results))
	}
}

func TestPolicyPersistsProviderFailureAgainstReservedCall(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, _ := policyTestSnapshot(t, projection)
	client := &policyTestClient{err: errors.New("provider offline")}
	journal := &policyTestCallJournal{}
	policy, err := New(client, policyTestAttestedBrain(), newPolicyTestProjectionLoader(projection), journal)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := policy.Decide(context.Background(), snapshot)
	if !errors.Is(err, ErrGeneration) {
		t.Fatalf("error=%v want ErrGeneration", err)
	}
	if !outcome.ProviderRequestDispatched {
		t.Fatal("provider invocation failure was not reported")
	}
	if client.generateCalls != 1 || len(journal.results) != 1 ||
		journal.results[0].Status != CallResultFailed || journal.results[0].FailureCode != CallFailureGeneration {
		t.Fatalf("provider failure journal=%#v generate=%d", journal.results, client.generateCalls)
	}
	if err := journal.results[0].Validate(journal.attempts[0]); err != nil {
		t.Fatalf("failed result: %v", err)
	}
}

func TestPolicyJournalFailureNeverReturnsAResult(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, _ := policyTestSnapshot(t, projection)
	client := &policyTestClient{err: errors.New("provider offline")}
	journal := &policyTestCallJournal{finishErr: errors.New("commit failed")}
	policy, err := New(client, policyTestAttestedBrain(), newPolicyTestProjectionLoader(projection), journal)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := policy.Decide(context.Background(), snapshot)
	if !errors.Is(err, ErrGeneration) || !errors.Is(err, ErrCallJournal) {
		t.Fatalf("error=%v want joined generation and journal failures", err)
	}
	decision := outcome.Decision
	if !outcome.ProviderRequestDispatched {
		t.Fatal("failed provider invocation was hidden by the journal error")
	}
	if decision.ObligationID != "" || decision.Action.Kind != "" {
		t.Fatalf("decision escaped failed journal: %#v", decision)
	}
}

func TestPolicyReplaysAcceptedCallWithoutInference(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, evidence := policyTestSnapshot(t, projection)
	firstClient := &policyTestClient{response: policyTestResponse(t, snapshot, evidence)}
	firstJournal := &policyTestCallJournal{}
	first, err := New(firstClient, policyTestAttestedBrain(), newPolicyTestProjectionLoader(projection), firstJournal)
	if err != nil {
		t.Fatal(err)
	}
	want, err := first.Decide(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	attempt, result, responseEvidence := firstJournal.attempts[0], firstJournal.results[0], firstJournal.evidence[0]
	replayJournal := &policyTestCallJournal{reservation: &CallReservation{
		Attempt: attempt, ExistingResult: &result, ExistingResponseEvidence: &responseEvidence,
		Created: false,
	}}
	replayClient := &policyTestClient{err: errors.New("must not be called")}
	replay, err := New(replayClient, policyTestAttestedBrain(), newPolicyTestProjectionLoader(projection), replayJournal)
	if err != nil {
		t.Fatal(err)
	}
	got, err := replay.Decide(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !want.ProviderRequestDispatched || got.ProviderRequestDispatched ||
		got.Decision.ObligationID != want.Decision.ObligationID ||
		got.Decision.Action.Kind != want.Decision.Action.Kind || replayClient.generateCalls != 0 {
		t.Fatalf("replay=%#v model calls=%d", got, replayClient.generateCalls)
	}
}

func TestPolicyRefusesIndeterminateAndRejectedReplayWithoutInference(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, _ := policyTestSnapshot(t, projection)
	envelope, err := Render(snapshot, projection, policyTestBrain())
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := newCallAttempt(snapshot, policyTestAttestedBrain(), envelope)
	if err != nil {
		t.Fatal(err)
	}
	rejectedGeneration := policyTestPreparedGeneration(attempt, "not-json")
	authorityGeneration := policyTestPreparedGeneration(attempt, `{"complete":true}`)
	for name, reservation := range map[string]struct {
		value CallReservation
		want  error
	}{
		"indeterminate": {CallReservation{Attempt: attempt}, ErrCallIndeterminate},
		"rejected": {CallReservation{Attempt: attempt, ExistingResult: callResultPointer(rejectedCallResult(
			attempt, rejectedGeneration, CallFailureInvalidDecision, errors.New("invalid response"),
		))}, ErrInvalidDecision},
		"authority_denied": {CallReservation{Attempt: attempt, ExistingResult: callResultPointer(rejectedCallResult(
			attempt, authorityGeneration, CallFailureAuthorityDenied,
			errors.Join(ErrInvalidDecision, cognition.ErrAuthorityDenied),
		))}, cognition.ErrAuthorityDenied},
		"failed": {CallReservation{Attempt: attempt, ExistingResult: callResultPointer(failedCallResult(
			attempt, policyTestFailedGeneration(attempt), errors.New("provider offline"),
		))}, ErrGeneration},
		"provider_identity_failed": {CallReservation{
			Attempt: attempt, ExistingResult: callResultPointer(providerIdentityFailedCallResult(
				attempt, policyTestFailedProviderIdentityGeneration(attempt),
				errors.New("provider identity changed"),
			)),
		}, ErrProviderIdentity},
	} {
		name, reservation := name, reservation
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client := &policyTestClient{err: errors.New("must not be called")}
			journal := &policyTestCallJournal{reservation: &reservation.value}
			policy, err := New(client, policyTestAttestedBrain(), newPolicyTestProjectionLoader(projection), journal)
			if err != nil {
				t.Fatal(err)
			}
			outcome, err := policy.Decide(context.Background(), snapshot)
			if !errors.Is(err, reservation.want) {
				t.Fatalf("error=%v want %v", err, reservation.want)
			}
			if name == "authority_denied" && !errors.Is(err, ErrInvalidDecision) {
				t.Fatalf("authority replay error=%v want ErrInvalidDecision too", err)
			}
			if outcome.ProviderRequestDispatched || client.generateCalls != 0 {
				t.Fatal("durable prior outcome reached model")
			}
		})
	}
}

func callResultPointer(result CallResult) *CallResult { return &result }
