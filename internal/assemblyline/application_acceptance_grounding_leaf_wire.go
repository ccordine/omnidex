package assemblyline

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type acceptanceGroundingLeaf struct {
	Field     string
	Site      AcceptanceObservationSite
	Criterion FrozenAcceptanceCriterion
}

type ApplicationAcceptanceGroundingLeafValidationKind string

const (
	ApplicationAcceptanceGroundingLeafAbsent     ApplicationAcceptanceGroundingLeafValidationKind = "absent"
	ApplicationAcceptanceGroundingLeafNonBoolean ApplicationAcceptanceGroundingLeafValidationKind = "non_boolean"
)

type ApplicationAcceptanceGroundingLeafValidationError struct {
	field string
	kind  ApplicationAcceptanceGroundingLeafValidationKind
}

func (failure *ApplicationAcceptanceGroundingLeafValidationError) Error() string {
	return fmt.Sprintf("acceptance grounding leaf %s is %s", failure.field, failure.kind)
}

func (failure *ApplicationAcceptanceGroundingLeafValidationError) Field() string {
	if failure == nil {
		return ""
	}
	return failure.field
}

func (failure *ApplicationAcceptanceGroundingLeafValidationError) Kind() ApplicationAcceptanceGroundingLeafValidationKind {
	if failure == nil {
		return ""
	}
	return failure.kind
}

func acceptanceGroundingLeafField(siteID string, criterionID string) string {
	return siteID + "__" + criterionID
}

func (input ApplicationAcceptanceGroundingReviewInput) groundingLeaves() []acceptanceGroundingLeaf {
	sites := input.reviewSites()
	leaves := make([]acceptanceGroundingLeaf, 0, len(sites)*len(input.Criteria))
	for _, site := range sites {
		for _, criterion := range input.Criteria {
			leaves = append(leaves, acceptanceGroundingLeaf{
				Field: acceptanceGroundingLeafField(site.ID, criterion.ID),
				Site:  site, Criterion: criterion,
			})
		}
	}
	return leaves
}

func acceptanceGroundingLeafDescription(leaf acceptanceGroundingLeaf) string {
	evidence, _ := json.Marshal(struct {
		Operations []string                       `json:"operations"`
		Literals   []AcceptanceObservationLiteral `json:"literals"`
	}{Operations: leaf.Site.Operations, Literals: leaf.Site.Literals})
	return fmt.Sprintf(
		"True only when %s (%s) authorizes every source-free operation and literal in %s: %s",
		leaf.Criterion.ID, leaf.Criterion.Statement, leaf.Site.ID, evidence,
	)
}

func ApplicationAcceptanceGroundingReviewResponseSchema(
	input ApplicationAcceptanceGroundingReviewInput,
) (map[string]any, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	leaves := input.groundingLeaves()
	properties := make(map[string]any, len(leaves))
	required := make([]string, len(leaves))
	for index, leaf := range leaves {
		required[index] = leaf.Field
		properties[leaf.Field] = map[string]any{
			"type": "boolean", "description": acceptanceGroundingLeafDescription(leaf),
		}
	}
	return map[string]any{
		"type": "object", "properties": properties, "required": required,
		"additionalProperties": false,
	}, nil
}

func DecodeApplicationAcceptanceGroundingReview(
	input ApplicationAcceptanceGroundingReviewInput,
	raw string,
) (ApplicationAcceptanceGroundingReview, error) {
	var zero ApplicationAcceptanceGroundingReview
	if err := input.validate(); err != nil {
		return zero, err
	}
	values, err := decodeApplicationAcceptanceGroundingLeaves(input, raw)
	if err != nil {
		return zero, err
	}
	review := deriveApplicationAcceptanceGroundingReview(input, values)
	if err := validateApplicationAcceptanceGroundingReview(input, review); err != nil {
		return zero, err
	}
	binding, err := acceptanceGroundingInputBinding(input)
	if err != nil {
		return zero, err
	}
	review.binding = binding
	return review, nil
}

func decodeApplicationAcceptanceGroundingLeaves(
	input ApplicationAcceptanceGroundingReviewInput,
	raw string,
) (map[string]bool, error) {
	fields, err := decodeJSONObject(raw, "application acceptance grounding review")
	if err != nil {
		return nil, err
	}
	leaves := input.groundingLeaves()
	known := make(map[string]acceptanceGroundingLeaf, len(leaves))
	values := make(map[string]bool, len(leaves))
	for _, leaf := range leaves {
		known[leaf.Field] = leaf
	}
	extra := make([]string, 0)
	for field := range fields {
		if _, exists := known[field]; !exists {
			extra = append(extra, field)
		}
	}
	if len(extra) != 0 {
		sort.Strings(extra)
		return nil, fmt.Errorf(
			"application acceptance grounding review contains unsupported fields: %s",
			strings.Join(extra, ", "),
		)
	}
	for _, leaf := range leaves {
		value, exists := fields[leaf.Field]
		if !exists {
			return nil, &ApplicationAcceptanceGroundingLeafValidationError{
				field: leaf.Field, kind: ApplicationAcceptanceGroundingLeafAbsent,
			}
		}
		boolean, ok := value.(bool)
		if !ok {
			return nil, &ApplicationAcceptanceGroundingLeafValidationError{
				field: leaf.Field, kind: ApplicationAcceptanceGroundingLeafNonBoolean,
			}
		}
		values[leaf.Field] = boolean
	}
	return values, nil
}

func deriveApplicationAcceptanceGroundingReview(
	input ApplicationAcceptanceGroundingReviewInput,
	values map[string]bool,
) ApplicationAcceptanceGroundingReview {
	mappings := make([]AcceptanceGroundingMapping, 0, len(input.reviewSites()))
	criterionUse := make(map[string]int, len(input.Criteria))
	for _, site := range input.reviewSites() {
		if stringInSet("untrusted_call", site.Operations) {
			return ApplicationAcceptanceGroundingReview{
				Decision: AcceptanceGroundingRepair, UnsupportedSiteID: site.ID,
			}
		}
		authorityIDs := make([]string, 0, len(input.Criteria))
		for _, criterion := range input.Criteria {
			if values[acceptanceGroundingLeafField(site.ID, criterion.ID)] {
				authorityIDs = append(authorityIDs, criterion.ID)
				criterionUse[criterion.ID]++
			}
		}
		if len(authorityIDs) == 0 {
			return ApplicationAcceptanceGroundingReview{
				Decision: AcceptanceGroundingRepair, UnsupportedSiteID: site.ID,
			}
		}
		mappings = append(mappings, AcceptanceGroundingMapping{
			SiteID: site.ID, AuthorityIDs: authorityIDs,
		})
	}
	for _, criterion := range input.Criteria {
		if criterionUse[criterion.ID] == 0 {
			return ApplicationAcceptanceGroundingReview{
				Decision: AcceptanceGroundingRepair, MissingCriterionID: criterion.ID,
			}
		}
	}
	return ApplicationAcceptanceGroundingReview{
		Decision: AcceptanceGroundingAccept, Mappings: mappings,
	}
}
