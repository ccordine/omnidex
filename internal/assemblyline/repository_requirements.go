package assemblyline

import (
	"fmt"
	"sort"
)

const RepositoryRequirementInterpretationSchemaV1 = "omnidex.repository-requirements.v1"

type RepositoryRequirementInterpretationInput struct {
	UserRequest string `json:"user_request"`
}

type RepositoryRequirementInterpretation struct {
	Schema        string   `json:"schema"`
	FeatureQuotes []string `json:"feature_quotes"`
}

func (input RepositoryRequirementInterpretationInput) validate() error {
	return validateApplicationRequest("repository requirements", input.UserRequest)
}

func ResolveRepositoryRequirements(
	input RepositoryRequirementInterpretationInput,
	interpretation RepositoryRequirementInterpretation,
) ([]string, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	if interpretation.Schema != RepositoryRequirementInterpretationSchemaV1 {
		return nil, fmt.Errorf(
			"repository requirement schema must be %q",
			RepositoryRequirementInterpretationSchemaV1,
		)
	}
	if interpretation.FeatureQuotes == nil {
		return nil, fmt.Errorf("repository feature quotes must be an array")
	}
	if len(interpretation.FeatureQuotes) < 1 || len(interpretation.FeatureQuotes) > maxRequirementCount {
		return nil, fmt.Errorf(
			"repository requirements must contain between 1 and %d feature quotes",
			maxRequirementCount,
		)
	}

	type groundedQuote struct {
		quote string
		span  textSpan
	}
	grounded := make([]groundedQuote, 0, len(interpretation.FeatureQuotes))
	seen := make(map[string]struct{}, len(interpretation.FeatureQuotes))
	for index, quote := range interpretation.FeatureQuotes {
		label := fmt.Sprintf("repository feature quote %d", index)
		if err := validateRequirementQuote(label, quote); err != nil {
			return nil, err
		}
		if _, duplicate := seen[quote]; duplicate {
			return nil, fmt.Errorf("%s duplicates %q", label, quote)
		}
		seen[quote] = struct{}{}
		span, err := uniqueTextSpan(input.UserRequest, quote)
		if err != nil {
			return nil, fmt.Errorf("%s %q: %w", label, quote, err)
		}
		for _, prior := range grounded {
			if span.Overlaps(prior.span) {
				return nil, fmt.Errorf("%s %q overlaps %q", label, quote, prior.quote)
			}
		}
		grounded = append(grounded, groundedQuote{quote: quote, span: span})
	}
	sort.SliceStable(grounded, func(left, right int) bool {
		return grounded[left].span.Start < grounded[right].span.Start
	})
	quotes := make([]string, 0, len(grounded))
	for _, item := range grounded {
		quotes = append(quotes, item.quote)
	}
	return quotes, nil
}
