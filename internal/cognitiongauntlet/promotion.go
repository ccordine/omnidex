package cognitiongauntlet

import "fmt"

type CompetencePolicy string

const (
	CompetenceSuccessSuperiority    CompetencePolicy = "success_superiority"
	CompetenceEfficiencySuperiority CompetencePolicy = "efficiency_superiority"
)

type CompetenceGateInput struct {
	Policy                     CompetencePolicy `json:"policy"`
	Tasks                      int              `json:"tasks"`
	PairedLiftLowerBoundPoints float64          `json:"paired_lift_lower_bound_points"`
	Rescues                    int              `json:"rescues"`
	Regressions                int              `json:"regressions"`
	ValidityReductionPoints    float64          `json:"validity_reduction_points"`
	SuccessLossPoints          float64          `json:"success_loss_points"`
	ContextReduction           float64          `json:"context_reduction"`
	DuplicateAcquisitionDelta  int              `json:"duplicate_acquisition_delta"`
	ToolOperationDelta         int              `json:"tool_operation_delta"`
	RequiredContextReduction   float64          `json:"required_context_reduction"`
}

func EvaluateCompetenceGate(input CompetenceGateInput) GateResult {
	reasons := []string{}
	if !finite(input.PairedLiftLowerBoundPoints) || !finite(input.ValidityReductionPoints) ||
		!finite(input.SuccessLossPoints) || !finite(input.ContextReduction) ||
		!finite(input.RequiredContextReduction) {
		return GateResult{Passed: false, Reasons: []string{"competence gate inputs must be finite"}}
	}
	if input.Tasks <= 0 {
		reasons = append(reasons, "competence gate requires at least one paired task")
	}
	if input.Rescues < 0 || input.Regressions < 0 || input.Rescues+input.Regressions > input.Tasks {
		reasons = append(reasons, "paired rescue/regression counts are invalid")
	}
	if input.ValidityReductionPoints > 0 {
		reasons = append(reasons, "candidate reduced validity")
	}
	switch input.Policy {
	case CompetenceSuccessSuperiority:
		if input.PairedLiftLowerBoundPoints <= 0 {
			reasons = append(reasons, "paired success lift is not credibly positive")
		}
		if input.Rescues <= input.Regressions {
			reasons = append(reasons, "rescues do not materially exceed regressions")
		}
	case CompetenceEfficiencySuperiority:
		if input.SuccessLossPoints < 0 || input.SuccessLossPoints > 2 {
			reasons = append(reasons, "success loss exceeds two percentage points")
		}
		if input.RequiredContextReduction < 0.40 || input.RequiredContextReduction > 0.50 {
			reasons = append(reasons, "pre-registered context reduction must be between 40% and 50%")
		} else if input.ContextReduction < input.RequiredContextReduction {
			reasons = append(reasons, fmt.Sprintf("context reduction %.3f is below required %.3f", input.ContextReduction, input.RequiredContextReduction))
		}
		if input.DuplicateAcquisitionDelta >= 0 || input.ToolOperationDelta >= 0 {
			reasons = append(reasons, "candidate did not reduce duplicate acquisitions and tool operations")
		}
	default:
		reasons = append(reasons, "competence policy is unregistered")
	}
	return GateResult{Passed: len(reasons) == 0, Reasons: reasons}
}

type TransferGateInput struct {
	HeldOutSurfaces         []string `json:"held_out_surfaces"`
	SuccessfulSurfaces      []string `json:"successful_surfaces"`
	ProductionSourceChanges int      `json:"production_source_changes"`
	RendererChanges         int      `json:"renderer_changes"`
	PolicyChanges           int      `json:"policy_changes"`
	PromptChanges           int      `json:"prompt_changes"`
}

func EvaluateTransferGate(input TransferGateInput) GateResult {
	reasons := []string{}
	heldOut, err := exactUniqueSet(input.HeldOutSurfaces, "held-out surface")
	if err != nil {
		reasons = append(reasons, err.Error())
	}
	successful, err := exactUniqueSet(input.SuccessfulSurfaces, "successful surface")
	if err != nil {
		reasons = append(reasons, err.Error())
	}
	if len(heldOut) < 2 {
		reasons = append(reasons, "transfer gate requires at least two held-out surfaces")
	}
	for surface := range heldOut {
		if _, exists := successful[surface]; !exists {
			reasons = append(reasons, fmt.Sprintf("held-out surface %q did not succeed", surface))
		}
	}
	if input.ProductionSourceChanges != 0 || input.RendererChanges != 0 ||
		input.PolicyChanges != 0 || input.PromptChanges != 0 {
		reasons = append(reasons, "transfer run changed production cognition authority")
	}
	return GateResult{Passed: len(reasons) == 0, Reasons: reasons}
}

func exactUniqueSet(values []string, label string) (map[string]struct{}, error) {
	if values == nil {
		return nil, fmt.Errorf("%s list must be explicit", label)
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := requireExact(value, label, 256); err != nil {
			return nil, err
		}
		if _, duplicate := set[value]; duplicate {
			return nil, fmt.Errorf("%s %q is duplicated", label, value)
		}
		set[value] = struct{}{}
	}
	return set, nil
}
