package assemblyline

import "fmt"

func AcceptApplicationAcceptanceGroundingReview(
	input ApplicationAcceptanceGroundingReviewInput,
	review ApplicationAcceptanceGroundingReview,
) (ApplicationAcceptanceGroundingReceipt, error) {
	var zero ApplicationAcceptanceGroundingReceipt
	if err := input.validate(); err != nil {
		return zero, err
	}
	if review.Decision != AcceptanceGroundingAccept {
		return zero, fmt.Errorf("acceptance grounding receipt requires an accepted review")
	}
	if err := validateApplicationAcceptanceGroundingReview(input, review); err != nil {
		return zero, err
	}
	binding, err := acceptanceGroundingInputBinding(input)
	if err != nil {
		return zero, err
	}
	if review.binding != binding {
		return zero, fmt.Errorf("acceptance grounding review is not bound to the current authority")
	}
	mappings, err := input.completeReceiptMappings(review.Mappings)
	if err != nil {
		return zero, err
	}
	return ApplicationAcceptanceGroundingReceipt{
		Schema:         AcceptanceGroundingReceiptSchemaV1,
		WorkloadSHA256: input.WorkloadSHA256, TaskID: input.TaskID,
		SourceSHA256: input.SourceSHA256, InventorySHA256: input.Inventory.InventorySHA256,
		Mappings: mappings, BindingSHA256: binding,
	}, nil
}

func (receipt ApplicationAcceptanceGroundingReceipt) ValidateFor(
	input ApplicationAcceptanceGroundingReviewInput,
	currentSource string,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if receipt.Schema != AcceptanceGroundingReceiptSchemaV1 {
		return fmt.Errorf("acceptance grounding receipt schema is invalid")
	}
	if err := input.validateSource(currentSource); err != nil {
		return fmt.Errorf("acceptance grounding receipt: %w", err)
	}
	binding, err := acceptanceGroundingInputBinding(input)
	if err != nil {
		return err
	}
	if receipt.WorkloadSHA256 != input.WorkloadSHA256 || receipt.TaskID != input.TaskID ||
		receipt.SourceSHA256 != input.SourceSHA256 ||
		receipt.InventorySHA256 != input.Inventory.InventorySHA256 ||
		receipt.BindingSHA256 != binding {
		return fmt.Errorf("acceptance grounding receipt is not bound to the current task authority")
	}
	if err := validateAcceptanceGroundingReceiptMappings(input, receipt.Mappings); err != nil {
		return fmt.Errorf("acceptance grounding receipt mappings: %w", err)
	}
	return nil
}

func (receipt ApplicationAcceptanceGroundingReceipt) AuthorizesFeatureFailureAt(
	input ApplicationAcceptanceGroundingReviewInput,
	currentSource string,
	tsx bool,
	line int,
	column int,
) (bool, error) {
	if tsx != input.TSX {
		return false, fmt.Errorf("acceptance grounding syntax mode differs from bound source")
	}
	if err := receipt.ValidateFor(input, currentSource); err != nil {
		return false, err
	}
	siteID, mapped, err := ResolveTypeScriptAcceptanceObservationSite(
		currentSource, tsx, line, column,
	)
	if err != nil || !mapped {
		return false, err
	}
	for _, mapping := range receipt.Mappings {
		if mapping.SiteID != siteID {
			continue
		}
		for _, authorityID := range mapping.AuthorityIDs {
			if input.hasCriterion(authorityID) {
				return true, nil
			}
		}
		return false, nil
	}
	return false, fmt.Errorf("acceptance grounding receipt omits resolved site %s", siteID)
}

func (input ApplicationAcceptanceGroundingReviewInput) modelAuthorities() []AcceptanceGroundingAuthority {
	authorities := make([]AcceptanceGroundingAuthority, 0, len(input.Criteria))
	for _, criterion := range input.Criteria {
		authorities = append(authorities, AcceptanceGroundingAuthority{
			ID: criterion.ID, Kind: AcceptanceGroundingCriterion, Statement: criterion.Statement,
			Operations: []string{},
		})
	}
	return authorities
}

func (input ApplicationAcceptanceGroundingReviewInput) platformAuthorityFor(operation string) string {
	for _, authority := range input.PlatformAuthorities {
		if stringInSet(operation, authority.Operations) {
			return authority.ID
		}
	}
	return ""
}

func (input ApplicationAcceptanceGroundingReviewInput) authorityOrder() map[string]int {
	authorities := input.modelAuthorities()
	order := make(map[string]int, len(authorities))
	for index, authority := range authorities {
		order[authority.ID] = index
	}
	return order
}

func (input ApplicationAcceptanceGroundingReviewInput) hasSite(id string) bool {
	for _, site := range input.Inventory.Sites {
		if site.ID == id {
			return true
		}
	}
	return false
}

func (input ApplicationAcceptanceGroundingReviewInput) hasCriterion(id string) bool {
	for _, criterion := range input.Criteria {
		if criterion.ID == id {
			return true
		}
	}
	return false
}

func cloneAcceptanceGroundingMappings(values []AcceptanceGroundingMapping) []AcceptanceGroundingMapping {
	if values == nil {
		return nil
	}
	result := make([]AcceptanceGroundingMapping, len(values))
	for index, value := range values {
		result[index] = AcceptanceGroundingMapping{
			SiteID:       value.SiteID,
			AuthorityIDs: append([]string(nil), value.AuthorityIDs...),
		}
	}
	return result
}
