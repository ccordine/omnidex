package cognition

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func testDecision(t *testing.T) (CognitionDecision, ActionSchema) {
	t.Helper()
	schema := testActionSchema(t, EvidenceRequired)
	evidence := testEvidenceRef(t)
	return CognitionDecision{
		ObligationID: "obligation-1",
		Action: ActionRequest{
			Kind: "inspect", Arguments: []ActionArgument{{Name: "target", Value: "entity-1"}},
		},
		EvidenceRefs:   []EvidenceRef{evidence},
		ExpectedEffect: "Expose the selected entity's current public properties.",
		Proposals: []LedgerProposal{{
			Kind: ProposalHypothesis, Content: "The selected entity may satisfy the obligation.",
			EvidenceRefs: []EvidenceRef{evidence},
		}},
		Attention: []AttentionRequest{{
			Operation: AttentionRetain, TargetRef: evidence,
			Scope: AttentionScopeObligation, Reason: "Needed by the active obligation.",
		}},
	}, schema
}

func TestDecodeCognitionDecisionRejectsDuplicateKeysAtEveryDepth(t *testing.T) {
	t.Parallel()
	decision, schema := testDecision(t)
	raw, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	valid := string(raw)
	duplicates := map[string]string{
		"top level": strings.Replace(
			valid, `{"obligation_id":`, `{"obligation_id":"obligation-shadow","obligation_id":`, 1,
		),
		"nested action": strings.Replace(
			valid, `"kind":"inspect"`, `"kind":"unregistered","kind":"inspect"`, 1,
		),
		"nested revision": strings.Replace(
			valid, `"number":1`, `"number":99,"number":1`, 1,
		),
	}
	for name, candidate := range duplicates {
		candidate := candidate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeCognitionDecision([]byte(candidate), schema); !errors.Is(err, ErrInvalidDecision) {
				t.Fatalf("duplicate key error = %v, want ErrInvalidDecision", err)
			}
		})
	}
}

func TestDecodeCognitionDecisionRequiresExactJSONFieldNames(t *testing.T) {
	t.Parallel()
	decision, schema := testDecision(t)
	raw, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	valid := string(raw)
	candidates := map[string]string{
		"top-level case alias": strings.Replace(
			valid, `"obligation_id":`, `"Obligation_ID":`, 1,
		),
		"nested action case alias": strings.Replace(
			valid, `"kind":"inspect"`, `"Kind":"inspect"`, 1,
		),
		"nested evidence case alias": strings.Replace(
			valid, `"observation_id":`, `"Observation_ID":`, 1,
		),
		"two case aliases for one field": strings.Replace(
			valid, `{"obligation_id":`, `{"Obligation_ID":"obligation-shadow","obligation_id":`, 1,
		),
	}
	for name, candidate := range candidates {
		candidate := candidate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodeCognitionDecision([]byte(candidate), schema); !errors.Is(err, ErrInvalidDecision) {
				t.Fatalf("inexact field error = %v, want ErrInvalidDecision", err)
			}
		})
	}
}

func TestDecodeCognitionDecisionRequiresExplicitArraysAndExactTaggedUnion(t *testing.T) {
	t.Parallel()
	schema, err := NewActionSchema(
		"action.observe.v1", "1.0.0", "observe", []ActionParameterSpec{}, EvidenceOptional,
	)
	if err != nil {
		t.Fatal(err)
	}
	missingOrNull := []string{
		`{"obligation_id":"obligation-1","action":{"kind":"observe"},"expected_effect":"Observe."}`,
		`{"obligation_id":"obligation-1","action":{"kind":"observe","arguments":null},"evidence_refs":null,"expected_effect":"Observe."}`,
	}
	for _, raw := range missingOrNull {
		if _, err := DecodeCognitionDecision([]byte(raw), schema); !errors.Is(err, ErrInvalidDecision) {
			t.Fatalf("implicit array contract error=%v, want ErrInvalidDecision", err)
		}
	}

	decision, requiredSchema := testDecision(t)
	raw, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	withForeignNull := strings.Replace(
		string(raw),
		`"kind":"hypothesis","content":`,
		`"kind":"hypothesis","obligation":null,"content":`,
		1,
	)
	if _, err := DecodeCognitionDecision([]byte(withForeignNull), requiredSchema); !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("tagged-union foreign field error=%v, want ErrInvalidDecision", err)
	}
}

func TestCognitionDecisionAllowsOnlyBoundedProposals(t *testing.T) {
	t.Parallel()
	decision, schema := testDecision(t)
	if err := decision.Validate(schema); err != nil {
		t.Fatalf("validate decision: %v", err)
	}

	for name, mutate := range map[string]func(*CognitionDecision){
		"missing obligation": func(value *CognitionDecision) { value.ObligationID = "" },
		"NUL effect":         func(value *CognitionDecision) { value.ExpectedEffect = "bad\x00effect" },
		"oversized effect": func(value *CognitionDecision) {
			value.ExpectedEffect = strings.Repeat("x", MaxExpectedEffectBytes+1)
		},
		"authoritative fact": func(value *CognitionDecision) {
			value.Proposals[0].Kind = LedgerProposalKind("fact")
		},
		"completion": func(value *CognitionDecision) {
			value.Proposals[0].Kind = LedgerProposalKind("completion")
		},
		"unknown attention": func(value *CognitionDecision) {
			value.Attention[0].Operation = AttentionOperation("pin_forever")
		},
	} {
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			candidate := decision.Clone()
			mutate(&candidate)
			if err := candidate.Validate(schema); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestDecodeCognitionDecisionRejectsCompletionAuthorityAndUnknownFields(t *testing.T) {
	t.Parallel()
	decision, schema := testDecision(t)
	raw := fmt.Sprintf(`{
		"obligation_id":"%s",
		"action":{"kind":"inspect","arguments":[{"name":"target","value":"entity-1"}]},
		"evidence_refs":[{"observation_id":"%s","revision":{"episode_id":"%s","number":1,"sha256":"%s"},"sha256":"%s"}],
		"expected_effect":"Inspect the target.",
		"complete":true
	}`, decision.ObligationID, decision.EvidenceRefs[0].ObservationID,
		decision.EvidenceRefs[0].Revision.EpisodeID, testDigest, decision.EvidenceRefs[0].SHA256)
	if _, err := DecodeCognitionDecision([]byte(raw), schema); !errors.Is(err, ErrAuthorityDenied) {
		t.Fatalf("completion field error = %v, want ErrAuthorityDenied", err)
	}
	caseAlias := strings.Replace(raw, `"complete":true`, `"Complete":true`, 1)
	if _, err := DecodeCognitionDecision([]byte(caseAlias), schema); !errors.Is(err, ErrAuthorityDenied) {
		t.Fatalf("completion alias error = %v, want ErrAuthorityDenied", err)
	}
	if err := ValidateCognitionDecisionAuthority([]byte(`{"completion":true}`)); !errors.Is(err, ErrAuthorityDenied) {
		t.Fatalf("authority preflight error = %v, want ErrAuthorityDenied", err)
	}

	raw = strings.Replace(raw, `"complete":true`, `"budget_increase":1000`, 1)
	if _, err := DecodeCognitionDecision([]byte(raw), schema); !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("unknown field error = %v, want ErrInvalidDecision", err)
	}
	if _, err := DecodeCognitionDecision([]byte(strings.Repeat(" ", MaxDecisionBytes+1)), schema); !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("oversized decision error = %v, want ErrInvalidDecision", err)
	}
}
