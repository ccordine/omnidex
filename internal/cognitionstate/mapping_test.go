package cognitionstate

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestEnvironmentObservationMapsOnlyToToolObservationWithExactProvenance(t *testing.T) {
	t.Parallel()
	schema := mappingTestSchema(t)
	action := mappingTestAction(t, schema)
	observation := mappingTestObservation(t, action.ID)
	input := EnvironmentObservationInput{
		Ledger: mappingTestLedger(t), Observation: observation,
		Action: &ActionBinding{Action: action, Schema: schema},
	}
	mutation, err := MapEnvironmentObservation(input)
	if err != nil {
		t.Fatalf("map observation: %v", err)
	}
	if err := mutation.Validate(); err != nil {
		t.Fatalf("validate mapping descriptor: %v", err)
	}
	command := mutation.Command()
	if command.Actor != taskstate.AuthorityToolEvidence || command.Kind != taskstate.EntryObservation ||
		command.Content != observation.Content || len(command.Refs) != 3 {
		t.Fatalf("observation command = %#v", command)
	}
	if command.Refs[0].Hash != observation.ContentSHA256 || command.Refs[0].Relation != taskstate.RefEvidence {
		t.Fatalf("observation evidence reference = %#v", command.Refs[0])
	}
	actionSHA, err := mappingDigest(action)
	if err != nil || command.Refs[2].Hash != actionSHA || command.Refs[2].URI == "" {
		t.Fatalf("action provenance = %#v, error=%v", command.Refs[2], err)
	}
	if descriptor := mutation.Descriptor(); descriptor.Actor != taskstate.AuthorityToolEvidence ||
		descriptor.SourceSHA256 == "" || descriptor.LedgerSHA256 == "" || descriptor.CommandSHA256 == "" {
		t.Fatalf("replay descriptor = %#v", descriptor)
	}
	tampered := mutation
	tampered.descriptor.LedgerSHA256 = strings.Repeat("f", 64)
	if err := tampered.Validate(); !errors.Is(err, ErrInvalidMapping) {
		t.Fatalf("tampered replay descriptor error = %v, want ErrInvalidMapping", err)
	}

	repeated, err := MapEnvironmentObservation(input)
	if err != nil || !reflect.DeepEqual(mutation.Descriptor(), repeated.Descriptor()) ||
		!reflect.DeepEqual(command, repeated.Command()) {
		t.Fatalf("mapping is not deterministic: %v", err)
	}
	ledger, err := taskstate.RestoreLedger(input.Ledger)
	if err != nil {
		t.Fatal(err)
	}
	first, err := ledger.Apply(command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ledger.Apply(command)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("exact replay diverged: %v", err)
	}
}

func TestEnvironmentObservationRejectsMissingOrMismatchedActionBinding(t *testing.T) {
	t.Parallel()
	schema := mappingTestSchema(t)
	action := mappingTestAction(t, schema)
	observation := mappingTestObservation(t, action.ID)
	if _, err := MapEnvironmentObservation(EnvironmentObservationInput{
		Ledger: mappingTestLedger(t), Observation: observation,
	}); !errors.Is(err, ErrInvalidMapping) {
		t.Fatalf("missing action error = %v, want ErrInvalidMapping", err)
	}
	other := action
	other.ID = "action-other"
	if _, err := MapEnvironmentObservation(EnvironmentObservationInput{
		Ledger: mappingTestLedger(t), Observation: observation,
		Action: &ActionBinding{Action: other, Schema: schema},
	}); !errors.Is(err, ErrInvalidMapping) {
		t.Fatalf("mismatched action error = %v, want ErrInvalidMapping", err)
	}
}

func TestActionFailureMapsOnlyToToolFailure(t *testing.T) {
	t.Parallel()
	schema := mappingTestSchema(t)
	action := mappingTestAction(t, schema)
	failure, err := cognition.NewActionFailure(
		cognition.ActionFailurePreconditionFailed, action, mappingTestRevision(),
		"The public precondition was not satisfied.", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := MapActionFailure(ActionFailureInput{
		Ledger: mappingTestLedger(t), Binding: ActionBinding{Action: action, Schema: schema},
		ExpectedRevision: mappingTestRevision(), Failure: failure,
	})
	if err != nil {
		t.Fatal(err)
	}
	command := mutation.Command()
	if command.Actor != taskstate.AuthorityToolEvidence || command.Kind != taskstate.EntryFailure ||
		len(command.Refs) < 3 {
		t.Fatalf("failure command = %#v", command)
	}
}

func TestModelProposalsRemainNonAuthoritativeAndObligationsStayCandidates(t *testing.T) {
	t.Parallel()
	observation := mappingTestObservation(t, "")
	evidence := observation.EvidenceRef()
	snapshot := mappingTestSnapshot(t, evidence)
	decision := cognition.CognitionDecision{
		ObligationID: snapshot.CurrentObligation().ID,
		Action:       cognition.ActionRequest{Kind: "inspect", Arguments: []cognition.ActionArgument{{Name: "target", Value: "entity-1"}}},
		EvidenceRefs: []cognition.EvidenceRef{evidence}, ExpectedEffect: "Expose public properties.",
		Proposals: []cognition.LedgerProposal{
			{Kind: cognition.ProposalObservation, Content: "A possible public pattern was observed.", EvidenceRefs: []cognition.EvidenceRef{evidence}},
			{Kind: cognition.ProposalHypothesis, Content: "The pattern may recur.", EvidenceRefs: []cognition.EvidenceRef{evidence}},
			{Kind: cognition.ProposalQuestion, Content: "Does the pattern recur?"},
			{Kind: cognition.ProposalObligation, Obligation: &cognition.ObligationProposal{
				Desired:      cognition.GoalExpression{All: []cognition.Predicate{{Name: "pattern.resolved", Args: []string{"target-41"}}}},
				EvidenceRefs: []cognition.EvidenceRef{evidence},
			}},
		},
	}
	mutations, err := MapModelProposals(ModelProposalInput{
		Ledger: mappingTestLedger(t), Snapshot: snapshot,
		Decision: decision, ActionSchema: mappingTestSchema(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []taskstate.EntryKind{
		taskstate.EntryObservation, taskstate.EntryHypothesis,
		taskstate.EntryQuestion, taskstate.EntryDecisionCandidate,
	}
	if len(mutations) != len(want) {
		t.Fatalf("mutation count = %d", len(mutations))
	}
	ledger, err := taskstate.RestoreLedger(mappingTestLedger(t))
	if err != nil {
		t.Fatal(err)
	}
	for index, mutation := range mutations {
		command := mutation.Command()
		if command.Actor != taskstate.AuthorityModelProposal || command.Kind != want[index] {
			t.Fatalf("proposal %d command = %#v", index, command)
		}
		if _, err := ledger.Apply(command); err != nil {
			t.Fatalf("apply proposal %d: %v", index, err)
		}
	}
	entry, _ := ledger.Entry(mutations[3].Command().ID)
	if entry.Kind != taskstate.EntryDecisionCandidate || entry.Authority != taskstate.AuthorityModelProposal {
		t.Fatalf("obligation proposal gained authority: %#v", entry)
	}
	if !strings.Contains(string(entry.Metadata.Bytes()), `"candidate_kind":"obligation"`) {
		t.Fatalf("obligation candidate metadata = %s", entry.Metadata.Bytes())
	}
}

func TestFactAcceptanceRequiresImmutableEvidenceAndRegisteredPolicy(t *testing.T) {
	t.Parallel()
	policy := FactAcceptancePolicyRef{ID: "fact-policy-1", Version: "1.0.0", SHA256: mappingTestDigest}
	registered, err := NewFactAcceptancePolicy(policy, func(evidence []FactEvidence) (string, error) {
		if len(evidence) != 1 {
			return "", fmt.Errorf("expected one observation")
		}
		return "accepted: " + evidence[0].Content, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewFactPolicyRegistry([]FactAcceptancePolicy{registered})
	if err != nil {
		t.Fatal(err)
	}
	observation := mappingTestObservation(t, "")
	observationMutation, err := MapEnvironmentObservation(EnvironmentObservationInput{
		Ledger: mappingTestLedger(t), Observation: observation,
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := taskstate.RestoreLedger(mappingTestLedger(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(observationMutation.Command()); err != nil {
		t.Fatal(err)
	}
	evidence := observation.EvidenceRef()
	input := FactAcceptanceInput{
		Ledger: ledger.MaterializedState(), EvidenceRefs: []cognition.EvidenceRef{evidence},
		PolicyID: policy.ID,
	}
	mutation, err := registry.MapAcceptedFact(input)
	if err != nil {
		t.Fatal(err)
	}
	command := mutation.Command()
	if command.Actor != taskstate.AuthorityCode || command.Kind != taskstate.EntryFact || len(command.Refs) != 1 {
		t.Fatalf("fact command = %#v", command)
	}
	if command.Content != "accepted: The public value is amber." {
		t.Fatalf("fact content = %q", command.Content)
	}
	input.PolicyID = "fact-policy-other"
	if _, err := registry.MapAcceptedFact(input); !errors.Is(err, ErrPolicyNotRegistered) {
		t.Fatalf("unregistered policy error = %v, want ErrPolicyNotRegistered", err)
	}
	input.PolicyID = policy.ID
	input.EvidenceRefs[0].SHA256 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err := registry.MapAcceptedFact(input); !errors.Is(err, ErrImmutableEvidence) {
		t.Fatalf("altered evidence error = %v, want ErrImmutableEvidence", err)
	}
}

func TestFactAcceptancePolicyRejectsInvalidRegistrationAndDerivation(t *testing.T) {
	t.Parallel()
	ref := FactAcceptancePolicyRef{ID: "fact-policy-1", Version: "1.0.0", SHA256: mappingTestDigest}
	if _, err := NewFactAcceptancePolicy(ref, nil); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("nil derivation error = %v, want ErrInvalidPolicy", err)
	}
	policy, err := NewFactAcceptancePolicy(ref, func([]FactEvidence) (string, error) {
		return "", errors.New("structured observation does not establish a fact")
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewFactPolicyRegistry([]FactAcceptancePolicy{policy})
	if err != nil {
		t.Fatal(err)
	}
	observation := mappingTestObservation(t, "")
	observationMutation, err := MapEnvironmentObservation(EnvironmentObservationInput{
		Ledger: mappingTestLedger(t), Observation: observation,
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := taskstate.RestoreLedger(mappingTestLedger(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(observationMutation.Command()); err != nil {
		t.Fatal(err)
	}
	_, err = registry.MapAcceptedFact(FactAcceptanceInput{
		Ledger: ledger.MaterializedState(), PolicyID: ref.ID,
		EvidenceRefs: []cognition.EvidenceRef{observation.EvidenceRef()},
	})
	if !errors.Is(err, ErrFactPolicyRejected) {
		t.Fatalf("derivation error = %v, want ErrFactPolicyRejected", err)
	}
}

func TestFactAcceptanceInputHasNoCallerAuthoredFactOrPolicyAuthority(t *testing.T) {
	t.Parallel()
	typeOf := reflect.TypeOf(FactAcceptanceInput{})
	for _, forbidden := range []string{"Content", "Policy", "PolicyRef"} {
		if _, exists := typeOf.FieldByName(forbidden); exists {
			t.Fatalf("FactAcceptanceInput exposes forbidden caller authority %q", forbidden)
		}
	}
	if field, exists := typeOf.FieldByName("PolicyID"); !exists ||
		field.Type != reflect.TypeOf(FactAcceptancePolicyID("")) {
		t.Fatalf("FactAcceptanceInput lacks exact registered policy identity")
	}
}
