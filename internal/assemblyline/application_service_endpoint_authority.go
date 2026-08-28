package assemblyline

type ApplicationServiceEndpointTaskAuthority struct {
	ProductContext    string   `json:"product_context"`
	RequirementQuote  string   `json:"requirement_quote"`
	Objective         string   `json:"objective"`
	RequiredBehaviors []string `json:"required_behaviors"`
}

func ProjectApplicationServiceEndpointTaskAuthority(
	authority ApplicationTaskRuntimeAuthority,
) (ApplicationServiceEndpointTaskAuthority, error) {
	projected := ApplicationServiceEndpointTaskAuthority{
		ProductContext: authority.ProductQuote, RequirementQuote: authority.RequirementQuote,
		Objective:         authority.Objective,
		RequiredBehaviors: append([]string(nil), authority.RequiredBehaviors...),
	}
	if err := projected.validate(); err != nil {
		return ApplicationServiceEndpointTaskAuthority{}, err
	}
	return projected, nil
}

func (authority ApplicationServiceEndpointTaskAuthority) validate() error {
	if err := validateApplicationProductQuote(
		"service endpoint product", authority.ProductContext,
	); err != nil {
		return err
	}
	if err := validateApplicationIntentText(
		"service endpoint requirement", authority.RequirementQuote, maxRequirementQuoteBytes,
	); err != nil {
		return err
	}
	if err := validateApplicationWorkloadLine(
		"service endpoint objective", authority.Objective, maxApplicationObjectiveRunes,
	); err != nil {
		return err
	}
	if err := validateApplicationJobSpecificationList(
		"service endpoint required behavior", authority.RequiredBehaviors,
		maxApplicationRequiredBehaviors, maxApplicationBehaviorRunes,
	); err != nil {
		return err
	}
	return nil
}
