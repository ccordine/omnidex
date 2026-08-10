package cognition

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestObligationProposalCarriesOnlyTypedSemanticsAndEvidence(t *testing.T) {
	t.Parallel()
	evidence := testEvidenceRef(t)
	desired := testGoalExpression(t, "condition.subgoal")
	proposal := LedgerProposal{
		Kind: ProposalObligation,
		Obligation: &ObligationProposal{
			Desired: desired, EvidenceRefs: []EvidenceRef{evidence},
		},
	}
	decision, schema := testDecision(t)
	decision.Proposals = []LedgerProposal{proposal}
	if err := decision.Validate(schema); err != nil {
		t.Fatalf("validate typed obligation proposal: %v", err)
	}

	for name, mutate := range map[string]func(*LedgerProposal){
		"missing typed semantics": func(value *LedgerProposal) { value.Obligation = nil },
		"ambiguous free text":     func(value *LedgerProposal) { value.Content = "Interpret this as a subgoal." },
		"duplicate evidence rail": func(value *LedgerProposal) {
			value.EvidenceRefs = []EvidenceRef{evidence}
		},
		"missing grounding": func(value *LedgerProposal) { value.Obligation.EvidenceRefs = nil },
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			candidate := decision.Clone()
			mutate(&candidate.Proposals[0])
			if err := candidate.Validate(schema); !errors.Is(err, ErrInvalidDecision) {
				t.Fatalf("error = %v, want ErrInvalidDecision", err)
			}
		})
	}

	duplicate := decision.Clone()
	duplicate.Proposals = append(duplicate.Proposals, duplicate.Proposals[0])
	if err := duplicate.Validate(schema); !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("multiple obligation proposals error = %v, want ErrInvalidDecision", err)
	}

	clone := decision.Clone()
	clone.Proposals[0].Obligation.Desired.All[0].Args[0] = "changed"
	if got := decision.Proposals[0].Obligation.Desired.All[0].Args[0]; got == "changed" {
		t.Fatal("decision clone retained obligation proposal storage")
	}
}

func TestDecodeObligationProposalRejectsModelOwnedLifecycleFields(t *testing.T) {
	t.Parallel()
	evidence := testEvidenceRef(t)
	decision, schema := testDecision(t)
	decision.Proposals = []LedgerProposal{{
		Kind: ProposalObligation,
		Obligation: &ObligationProposal{
			Desired:      testGoalExpression(t, "condition.subgoal"),
			EvidenceRefs: []EvidenceRef{evidence},
		},
	}}
	raw, err := json.Marshal(decision)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		`"id":"model-selected",`,
		`"status":"active",`,
		`"completion_check":{"id":"model-check","version":"1","sha256":"` + testDigest + `"},`,
	} {
		field := field
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			candidate := strings.Replace(string(raw), `"desired":`, field+`"desired":`, 1)
			if _, err := DecodeCognitionDecision([]byte(candidate), schema); !errors.Is(err, ErrInvalidDecision) {
				t.Fatalf("model lifecycle field error = %v, want ErrInvalidDecision", err)
			}
		})
	}
}

func TestNonObligationProposalRejectsTypedObligationPayload(t *testing.T) {
	t.Parallel()
	decision, schema := testDecision(t)
	decision.Proposals[0].Obligation = &ObligationProposal{
		Desired:      testGoalExpression(t, "condition.unexpected"),
		EvidenceRefs: []EvidenceRef{decision.EvidenceRefs[0]},
	}
	if err := decision.Validate(schema); !errors.Is(err, ErrInvalidDecision) {
		t.Fatalf("error = %v, want ErrInvalidDecision", err)
	}
}
