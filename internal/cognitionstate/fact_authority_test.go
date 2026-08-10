package cognitionstate

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestFactAcceptanceAuthorityPlansOnlyExactTransitionEvidence(t *testing.T) {
	ledger, node, observation, transition := factAuthorityFixture(t)
	policyRef := FactAcceptancePolicyRef{
		ID: "policy.accept.public-state", Version: "1.0.0", SHA256: strings.Repeat("a", 64),
	}
	policy, err := NewFactAcceptancePolicy(policyRef, func(evidence []FactEvidence) (string, error) {
		return "accepted:" + evidence[0].Content, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewFactPolicyRegistry([]FactAcceptancePolicy{policy})
	if err != nil {
		t.Fatal(err)
	}
	authority, err := NewFactAcceptanceAuthority(
		factPlannerStub{ref: FactPlannerRef{ID: "planner.public-state", Version: "1.0.0", SHA256: strings.Repeat("b", 64)},
			plan: func(cognition.Transition) ([]FactPlan, error) {
				return []FactPlan{{PolicyID: policyRef.ID, EvidenceRefs: []cognition.EvidenceRef{observation.EvidenceRef()}}}, nil
			}}, registry,
	)
	if err != nil {
		t.Fatal(err)
	}
	mutations, err := authority.MapTransitionFacts(ledger, node, transition)
	if err != nil || len(mutations) != 1 || mutations[0].Command().Content != "accepted:"+observation.Content {
		t.Fatalf("fact mutations=%+v error=%v", mutations, err)
	}
	ref := authority.Reference()
	if ref.Policies == nil || len(ref.Policies) != 1 || ref.Policies[0] != policyRef {
		t.Fatalf("authority reference=%+v", ref)
	}
	changed := transition
	changed.Observations = []cognition.Observation{}
	if _, err := authority.MapTransitionFacts(ledger, node, changed); !errors.Is(err, ErrImmutableEvidence) {
		t.Fatalf("missing planned evidence error=%v", err)
	}
}

func TestNoFactAcceptanceAuthorityIsCanonicalAndExplicitEmpty(t *testing.T) {
	ledger, node, _, transition := factAuthorityFixture(t)
	left := NewNoFactAcceptanceAuthority()
	right := NewNoFactAcceptanceAuthority()
	if !reflect.DeepEqual(left.Reference(), right.Reference()) || left.Reference().Policies == nil {
		t.Fatalf("no-fact authority is not canonical explicit-empty: %+v %+v", left.Reference(), right.Reference())
	}
	mutations, err := left.MapTransitionFacts(ledger, node, transition)
	if err != nil || mutations == nil || len(mutations) != 0 {
		t.Fatalf("no-fact mutations=%+v error=%v", mutations, err)
	}
}

func TestFactAcceptanceAuthorityRejectsNilOrUnregisteredPlannerOutput(t *testing.T) {
	ledger, node, observation, transition := factAuthorityFixture(t)
	policy, err := NewFactAcceptancePolicy(FactAcceptancePolicyRef{
		ID: "policy.known", Version: "1", SHA256: strings.Repeat("c", 64),
	}, func([]FactEvidence) (string, error) { return "known", nil })
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewFactPolicyRegistry([]FactAcceptancePolicy{policy})
	if err != nil {
		t.Fatal(err)
	}
	for name, plan := range map[string]func(cognition.Transition) ([]FactPlan, error){
		"nil-output": func(cognition.Transition) ([]FactPlan, error) { return nil, nil },
		"unknown-policy": func(cognition.Transition) ([]FactPlan, error) {
			return []FactPlan{{PolicyID: "policy.unknown", EvidenceRefs: []cognition.EvidenceRef{observation.EvidenceRef()}}}, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			authority, err := NewFactAcceptanceAuthority(factPlannerStub{ref: FactPlannerRef{
				ID: "planner." + name, Version: "1", SHA256: strings.Repeat("d", 64),
			}, plan: plan}, registry)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := authority.MapTransitionFacts(ledger, node, transition); err == nil {
				t.Fatal("invalid planner output was accepted")
			}
		})
	}
}

func TestFactAcceptanceAuthorityRejectsChangedPlannerImplementationReference(t *testing.T) {
	policy, err := NewFactAcceptancePolicy(FactAcceptancePolicyRef{
		ID: "policy.stable", Version: "1", SHA256: strings.Repeat("e", 64),
	}, func([]FactEvidence) (string, error) { return "stable", nil })
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewFactPolicyRegistry([]FactAcceptancePolicy{policy})
	if err != nil {
		t.Fatal(err)
	}
	planner := &mutableFactPlanner{ref: FactPlannerRef{
		ID: "planner.mutable", Version: "1", SHA256: strings.Repeat("f", 64),
	}}
	authority, err := NewFactAcceptanceAuthority(planner, registry)
	if err != nil {
		t.Fatal(err)
	}
	planner.ref.Version = "2"
	if err := authority.Validate(); !errors.Is(err, ErrInvalidPolicy) {
		t.Fatalf("changed executable planner reference error=%v", err)
	}
}

type mutableFactPlanner struct{ ref FactPlannerRef }

func (planner *mutableFactPlanner) Reference() FactPlannerRef { return planner.ref }
func (*mutableFactPlanner) Plan(cognition.Transition) ([]FactPlan, error) {
	return []FactPlan{}, nil
}

type factPlannerStub struct {
	ref  FactPlannerRef
	plan func(cognition.Transition) ([]FactPlan, error)
}

func (planner factPlannerStub) Reference() FactPlannerRef { return planner.ref }

func (planner factPlannerStub) Plan(transition cognition.Transition) ([]FactPlan, error) {
	return planner.plan(transition)
}

func factAuthorityFixture(
	t *testing.T,
) (taskstate.MaterializedState, taskstate.NodeID, cognition.Observation, cognition.Transition) {
	t.Helper()
	observation := mappingTestObservation(t, "")
	mutation, err := MapEnvironmentObservation(EnvironmentObservationInput{
		Ledger: mappingTestLedger(t), Observation: observation,
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := taskstate.RestoreLedger(mappingTestLedger(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(mutation.Command()); err != nil {
		t.Fatal(err)
	}
	return ledger.MaterializedState(), "", observation, cognition.Transition{
		Current: observation.Revision, Observations: []cognition.Observation{observation}, Effects: []cognition.Effect{},
	}
}
