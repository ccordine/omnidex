package assemblyline

func (input ApplicationAcceptanceGroundingReviewInput) reviewSites() []AcceptanceObservationSite {
	sites := make([]AcceptanceObservationSite, 0, len(input.Inventory.Sites))
	for _, site := range input.Inventory.Sites {
		if operation, pure := acceptancePurePlatformSite(site); pure &&
			input.platformAuthorityFor(operation) != "" {
			continue
		}
		sites = append(sites, site)
	}
	return sites
}

func (input ApplicationAcceptanceGroundingReviewInput) reviewStatements() []AcceptanceObservationStatement {
	required := make(map[string]struct{})
	for _, site := range input.reviewSites() {
		required[site.StatementID] = struct{}{}
	}
	statements := make([]AcceptanceObservationStatement, 0, len(required))
	for _, statement := range input.Inventory.Statements {
		if _, exists := required[statement.ID]; exists {
			statements = append(statements, statement)
		}
	}
	return statements
}

func (input ApplicationAcceptanceGroundingReviewInput) deterministicHarnessMappings() (
	map[string]AcceptanceGroundingMapping,
	error,
) {
	mappings := make(map[string]AcceptanceGroundingMapping)
	for _, site := range input.Inventory.Sites {
		operation, pure := acceptancePurePlatformSite(site)
		if !pure {
			continue
		}
		owner := input.platformAuthorityFor(operation)
		if owner == "" {
			continue
		}
		mappings[site.ID] = AcceptanceGroundingMapping{
			SiteID: site.ID, AuthorityIDs: []string{owner},
		}
	}
	return mappings, nil
}

func acceptancePurePlatformSite(site AcceptanceObservationSite) (string, bool) {
	if len(site.Operations) != 1 || len(site.Literals) != 0 ||
		!acceptancePlatformOperation(site.Operations[0]) {
		return "", false
	}
	return site.Operations[0], true
}

func (input ApplicationAcceptanceGroundingReviewInput) hasReviewSite(id string) bool {
	for _, site := range input.reviewSites() {
		if site.ID == id {
			return true
		}
	}
	return false
}

func (input ApplicationAcceptanceGroundingReviewInput) completeReceiptMappings(
	reviewMappings []AcceptanceGroundingMapping,
) ([]AcceptanceGroundingMapping, error) {
	deterministic, err := input.deterministicHarnessMappings()
	if err != nil {
		return nil, err
	}
	result := make([]AcceptanceGroundingMapping, 0, len(input.Inventory.Sites))
	reviewIndex := 0
	for _, site := range input.Inventory.Sites {
		if mapping, exists := deterministic[site.ID]; exists {
			result = append(result, cloneAcceptanceGroundingMappings([]AcceptanceGroundingMapping{mapping})[0])
			continue
		}
		if reviewIndex >= len(reviewMappings) || reviewMappings[reviewIndex].SiteID != site.ID {
			return nil, newAcceptanceInventoryError(
				"accepted review omits product observation site " + site.ID,
			)
		}
		result = append(result, cloneAcceptanceGroundingMappings(
			[]AcceptanceGroundingMapping{reviewMappings[reviewIndex]},
		)[0])
		reviewIndex++
	}
	if reviewIndex != len(reviewMappings) {
		return nil, newAcceptanceInventoryError("accepted review contains extra product observation mappings")
	}
	return result, nil
}

func validateAcceptanceGroundingReceiptMappings(
	input ApplicationAcceptanceGroundingReviewInput,
	mappings []AcceptanceGroundingMapping,
) error {
	if len(mappings) != len(input.Inventory.Sites) {
		return newAcceptanceInventoryError("receipt does not cover the complete observation inventory")
	}
	deterministic, err := input.deterministicHarnessMappings()
	if err != nil {
		return err
	}
	reviewMappings := make([]AcceptanceGroundingMapping, 0, len(input.reviewSites()))
	for index, site := range input.Inventory.Sites {
		mapping := mappings[index]
		if mapping.SiteID != site.ID {
			return newAcceptanceInventoryError("receipt observation mappings are not in canonical order")
		}
		if expected, exists := deterministic[site.ID]; exists {
			if !equalAcceptanceGroundingMapping(mapping, expected) {
				return newAcceptanceInventoryError(
					"receipt changed deterministic harness authority for " + site.ID,
				)
			}
			continue
		}
		reviewMappings = append(reviewMappings, mapping)
	}
	return validateApplicationAcceptanceGroundingReview(input, ApplicationAcceptanceGroundingReview{
		Decision: AcceptanceGroundingAccept, Mappings: reviewMappings,
	})
}

func equalAcceptanceGroundingMapping(
	left AcceptanceGroundingMapping,
	right AcceptanceGroundingMapping,
) bool {
	if left.SiteID != right.SiteID || len(left.AuthorityIDs) != len(right.AuthorityIDs) {
		return false
	}
	for index := range left.AuthorityIDs {
		if left.AuthorityIDs[index] != right.AuthorityIDs[index] {
			return false
		}
	}
	return true
}
