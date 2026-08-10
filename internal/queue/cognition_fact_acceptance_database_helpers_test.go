package queue

import (
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
)

type cognitionFactPlannerTest struct {
	ref  cognitionstate.FactPlannerRef
	plan func(cognition.Transition) ([]cognitionstate.FactPlan, error)
}

func (planner cognitionFactPlannerTest) Reference() cognitionstate.FactPlannerRef {
	return planner.ref
}

func (planner cognitionFactPlannerTest) Plan(
	transition cognition.Transition,
) ([]cognitionstate.FactPlan, error) {
	return planner.plan(transition)
}

func cognitionFactAuthorityForTest(
	t *testing.T,
	plan func(cognition.Transition) ([]cognitionstate.FactPlan, error),
	plannerDigest string,
) cognitionstate.FactAcceptanceAuthority {
	t.Helper()
	policyRef := cognitionstate.FactAcceptancePolicyRef{
		ID: "fact-policy.test", Version: "1.0.0", SHA256: cognitionTestDigest("d"),
	}
	policy, err := cognitionstate.NewFactAcceptancePolicy(
		policyRef,
		func(evidence []cognitionstate.FactEvidence) (string, error) {
			if len(evidence) != 1 {
				return "", fmt.Errorf("test fact policy requires one exact observation")
			}
			return "Accepted fact: " + evidence[0].Content, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := cognitionstate.NewFactPolicyRegistry([]cognitionstate.FactAcceptancePolicy{policy})
	if err != nil {
		t.Fatal(err)
	}
	authority, err := cognitionstate.NewFactAcceptanceAuthority(
		cognitionFactPlannerTest{
			ref: cognitionstate.FactPlannerRef{
				ID: "fact-planner.test", Version: "1.0.0", SHA256: plannerDigest,
			},
			plan: plan,
		},
		registry,
	)
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

func planFirstCognitionObservation(
	transition cognition.Transition,
) ([]cognitionstate.FactPlan, error) {
	if len(transition.Observations) == 0 {
		return []cognitionstate.FactPlan{}, nil
	}
	return []cognitionstate.FactPlan{{
		PolicyID: "fact-policy.test",
		EvidenceRefs: []cognition.EvidenceRef{
			transition.Observations[0].EvidenceRef(),
		},
	}}, nil
}
