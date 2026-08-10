package cognitionpolicy

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestCanonicalPolicyJSONIsStableAndDoesNotApplyHTMLEscaping(t *testing.T) {
	t.Parallel()
	raw, err := canonicalPolicyJSON(map[string]any{
		"z": "<>&é\u2028\n\x01/",
		"a": []any{json.Number("1"), true, nil, map[string]any{"b": json.Number("2"), "a": "x"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"a\":[1,true,null,{\"a\":\"x\",\"b\":2}],\"z\":\"<>&é\u2028\\n\\u0001/\"}"
	if string(raw) != want {
		t.Fatalf("canonical JSON mismatch\n got: %s\nwant: %s", raw, want)
	}
}

func TestCallAttemptBindsExactCleanDeskBeforeInference(t *testing.T) {
	t.Parallel()
	attempt := policyTestCallAttempt(t)
	if err := attempt.Validate(); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*CallAttempt){
		"actor":               func(value *CallAttempt) { value.Actor.WorkerID = "worker-other" },
		"snapshot":            func(value *CallAttempt) { value.SnapshotSHA256 = strings.Repeat("d", 64) },
		"revision":            func(value *CallAttempt) { value.ExpectedRevision.SHA256 = strings.Repeat("e", 64) },
		"obligation":          func(value *CallAttempt) { value.ObligationID = "obligation-other" },
		"budget":              func(value *CallAttempt) { value.RuntimeBudget.MaxOutputBytes-- },
		"projection":          func(value *CallAttempt) { value.ContextProjection.WorkingSetVersion++ },
		"brain":               func(value *CallAttempt) { value.Brain.Hardware = "other-hardware" },
		"envelope":            func(value *CallAttempt) { value.Envelope += " " },
		"prompt hint":         func(value *CallAttempt) { value.PromptHint += " " },
		"model visible bytes": func(value *CallAttempt) { value.ModelVisibleInputBytes++ },
		"response contract": func(value *CallAttempt) {
			value.ResponseContractSHA256 = strings.Repeat("f", 64)
		},
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			changed := attempt
			mutate(&changed)
			if changed.ID == callAttemptID(changed) || changed.Validate() == nil {
				t.Fatalf("call attempt mutation %q retained its identity", name)
			}
		})
	}
}

func TestCallResultsDistinguishAcceptedRejectedAndFailed(t *testing.T) {
	t.Parallel()
	attempt := policyTestCallAttempt(t)
	response := `{"obligation_id":"obligation-1"}`
	generation := policyTestPreparedGeneration(attempt, response)
	accepted := acceptedCallResult(
		attempt, generation, cognition.ActionSchemaRef{
			ID: "schema-1", Version: "v1", SHA256: strings.Repeat("a", 64),
		},
		strings.Repeat("b", 64),
	)
	if err := accepted.Validate(attempt); err != nil {
		t.Fatalf("accepted result: %v", err)
	}
	rejected := rejectedCallResult(
		attempt, generation, CallFailureInvalidDecision,
		fmt.Errorf("response did not match the registered decision schema"),
	)
	if err := rejected.Validate(attempt); err != nil {
		t.Fatalf("rejected result: %v", err)
	}
	failed := failedCallResult(
		attempt, policyTestFailedGeneration(attempt),
		fmt.Errorf("the model provider returned an error"),
	)
	if err := failed.Validate(attempt); err != nil {
		t.Fatalf("failed result: %v", err)
	}

	changed := accepted
	changed.CallID = "cognition_call_" + strings.Repeat("f", 64)
	if changed.Validate(attempt) == nil {
		t.Fatal("call result escaped its reserved attempt")
	}
	changed = accepted
	changed.Status = CallResultRejected
	if changed.Validate(attempt) == nil {
		t.Fatal("accepted decision was relabeled as rejected")
	}
}

func TestCallReservationMakesIndeterminateStartExplicit(t *testing.T) {
	t.Parallel()
	attempt := policyTestCallAttempt(t)
	created := CallReservation{Attempt: attempt, Created: true}
	if err := created.ValidateFor(attempt); err != nil {
		t.Fatal(err)
	}
	indeterminate := CallReservation{Attempt: attempt, Created: false}
	if err := indeterminate.ValidateFor(attempt); err != nil {
		t.Fatal(err)
	}
	invalid := created
	invalid.ExistingResult = &CallResult{}
	if invalid.ValidateFor(attempt) == nil {
		t.Fatal("new call reservation carried an impossible prior result")
	}
}

func policyTestCallAttempt(t *testing.T) CallAttempt {
	t.Helper()
	projection := policyTestProjection(t, "one disposable call")
	snapshot, _ := policyTestSnapshot(t, projection)
	envelope, err := Render(snapshot, projection, policyTestBrain())
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := newCallAttempt(snapshot, policyTestAttestedBrain(), envelope)
	if err != nil {
		t.Fatal(err)
	}
	return attempt
}
