package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

type acceptanceGroundingModelProjection struct {
	Authorities []AcceptanceGroundingAuthority   `json:"authorities"`
	Statements  []AcceptanceObservationStatement `json:"statements"`
	Sites       []AcceptanceObservationSite      `json:"sites"`
}

func BuildApplicationAcceptanceGroundingReviewPrompt(
	input ApplicationAcceptanceGroundingReviewInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection := acceptanceGroundingModelProjection{
		Authorities: input.modelAuthorities(),
		Statements:  input.reviewStatements(),
		Sites:       input.reviewSites(),
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("encode acceptance grounding projection: %w", err)
	}
	leaves := input.groundingLeaves()
	fields := make([]string, len(leaves))
	for index, leaf := range leaves {
		fields[index] = leaf.Field
	}
	fieldJSON, err := json.Marshal(fields)
	if err != nil {
		return "", fmt.Errorf("encode acceptance grounding leaf fields: %w", err)
	}
	return strings.Join([]string{
		"For every listed observation/criterion pair, return true only when the complete observation is authorized by that criterion; otherwise return false. One supported sibling does not authorize another sibling.",
		"Return one JSON object containing every listed boolean field exactly once.",
		"ACCEPTANCE_GROUNDING_LEAF_FIELDS_JSON:\n" + string(fieldJSON),
		"ACCEPTANCE_GROUNDING_INPUT_JSON:\n" + string(raw),
	}, "\n\n"), nil
}

func validateApplicationAcceptanceGroundingReview(
	input ApplicationAcceptanceGroundingReviewInput,
	review ApplicationAcceptanceGroundingReview,
) error {
	if review.Decision == AcceptanceGroundingRepair {
		if review.Mappings != nil || (review.UnsupportedSiteID == "") == (review.MissingCriterionID == "") {
			return fmt.Errorf("acceptance grounding repair requires exactly one unsupported site or missing criterion")
		}
		if review.UnsupportedSiteID != "" && !input.hasReviewSite(review.UnsupportedSiteID) {
			return fmt.Errorf("acceptance grounding repair cites unknown site %q", review.UnsupportedSiteID)
		}
		if review.MissingCriterionID != "" && !input.hasCriterion(review.MissingCriterionID) {
			return fmt.Errorf("acceptance grounding repair cites unknown criterion %q", review.MissingCriterionID)
		}
		return nil
	}
	if review.Decision != AcceptanceGroundingAccept {
		return fmt.Errorf("acceptance grounding decision %q is unsupported", review.Decision)
	}
	if review.UnsupportedSiteID != "" || review.MissingCriterionID != "" || review.Mappings == nil {
		return fmt.Errorf("accepted grounding review requires only complete mappings")
	}
	reviewSites := input.reviewSites()
	if len(review.Mappings) != len(reviewSites) {
		return fmt.Errorf("accepted grounding review must map exactly %d product sites", len(reviewSites))
	}
	criterionUse := make(map[string]int, len(input.Criteria))
	authorityOrder := input.authorityOrder()
	for index, mapping := range review.Mappings {
		site := reviewSites[index]
		if mapping.SiteID != site.ID {
			return fmt.Errorf("grounding mapping %d must name site %s", index, site.ID)
		}
		if len(mapping.AuthorityIDs) == 0 {
			return fmt.Errorf("grounding mapping for %s has no authority", site.ID)
		}
		if stringInSet("untrusted_call", site.Operations) {
			return fmt.Errorf("untrusted observation site %s cannot be accepted", site.ID)
		}
		last := -1
		hasCriterion := false
		for _, authorityID := range mapping.AuthorityIDs {
			position, exists := authorityOrder[authorityID]
			if !exists {
				return fmt.Errorf("grounding mapping for %s cites unknown authority %q", site.ID, authorityID)
			}
			if !input.hasCriterion(authorityID) {
				return fmt.Errorf("product observation site %s cites non-criterion authority %q", site.ID, authorityID)
			}
			if position <= last {
				return fmt.Errorf("grounding mapping for %s has duplicate or unordered authorities", site.ID)
			}
			last = position
			hasCriterion = true
			criterionUse[authorityID]++
		}
		if !hasCriterion {
			return fmt.Errorf("product observation site %s requires criterion authority", site.ID)
		}
	}
	for _, criterion := range input.Criteria {
		if criterionUse[criterion.ID] == 0 {
			return fmt.Errorf("accepted grounding review does not cover criterion %s", criterion.ID)
		}
	}
	return nil
}
