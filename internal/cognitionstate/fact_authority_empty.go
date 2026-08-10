package cognitionstate

import "github.com/gryph/omnidex/internal/cognition"

const noFactPlannerDigest = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

type noFactPlanner struct{}

func (noFactPlanner) Reference() FactPlannerRef {
	return FactPlannerRef{ID: "fact-planner.none", Version: "1.0.0", SHA256: noFactPlannerDigest}
}

func (noFactPlanner) Plan(cognition.Transition) ([]FactPlan, error) {
	return []FactPlan{}, nil
}

func NewNoFactAcceptanceAuthority() FactAcceptanceAuthority {
	value, err := newFactAcceptanceAuthority(
		noFactPlanner{}, FactPolicyRegistry{policies: map[FactAcceptancePolicyID]FactAcceptancePolicy{}},
		[]FactAcceptancePolicyRef{},
	)
	if err != nil {
		panic("construct canonical no-fact acceptance authority: " + err.Error())
	}
	return value
}
