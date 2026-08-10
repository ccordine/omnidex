package cognitionpolicy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestPolicyStrictlyRejectsUnknownCompletionAndUnboundEvidence(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, evidence := policyTestSnapshot(t, projection)
	valid := policyTestResponse(t, snapshot, evidence)
	unknownCompletion := strings.TrimSuffix(valid, "}") + `,"complete":true}`
	unbound := evidence
	unbound.ObservationID = "observation-other"
	unboundResponse := policyTestResponse(t, snapshot, unbound)

	for name, testCase := range map[string]struct {
		response        string
		wantAuthority   bool
		wantFailureCode CallFailureCode
	}{
		"completion authority": {unknownCompletion, true, CallFailureAuthorityDenied},
		"completion only":      {`{"complete":true}`, true, CallFailureAuthorityDenied},
		"unbound evidence":     {unboundResponse, false, CallFailureInvalidDecision},
	} {
		testCase := testCase
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client := &policyTestClient{response: testCase.response}
			journal := &policyTestCallJournal{}
			policy, err := New(client, policyTestAttestedBrain(), newPolicyTestProjectionLoader(projection), journal)
			if err != nil {
				t.Fatal(err)
			}
			_, err = policy.Decide(context.Background(), snapshot)
			if !errors.Is(err, ErrInvalidDecision) ||
				errors.Is(err, cognition.ErrAuthorityDenied) != testCase.wantAuthority {
				t.Fatalf("error = %v, want invalid=true authority=%t", err, testCase.wantAuthority)
			}
			if len(journal.attempts) != 1 || len(journal.results) != 1 ||
				journal.results[0].Status != CallResultRejected ||
				journal.results[0].FailureCode != testCase.wantFailureCode {
				t.Fatalf("invalid decision was not durably classified: %#v", journal.results)
			}
		})
	}
}
