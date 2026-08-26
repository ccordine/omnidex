package assemblyline

type ApplicationServiceEndpointTaskAuthority struct {
	ProductContext     string   `json:"product_context"`
	RequirementQuote   string   `json:"requirement_quote"`
	Objective          string   `json:"objective"`
	RequiredBehaviors  []string `json:"required_behaviors"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

func ProjectApplicationServiceEndpointTaskAuthority(
	productContext string,
	task FrozenApplicationTask,
) (ApplicationServiceEndpointTaskAuthority, error) {
	authority := ApplicationServiceEndpointTaskAuthority{
		ProductContext: productContext, RequirementQuote: task.RequirementQuote,
		Objective:          task.Objective,
		RequiredBehaviors:  append([]string(nil), task.RequiredBehaviors...),
		AcceptanceCriteria: append([]string(nil), task.AcceptanceCriteria...),
	}
	if err := authority.validate(); err != nil {
		return ApplicationServiceEndpointTaskAuthority{}, err
	}
	return authority, nil
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
	return validateApplicationJobSpecificationList(
		"service endpoint acceptance criterion", authority.AcceptanceCriteria,
		maxApplicationAcceptanceCriteria, maxApplicationCriterionRunes,
	)
}
