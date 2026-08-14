package assemblyline

import (
	"fmt"
	"sort"
	"strings"
)

const ApplicationRequirementInterpretationSchemaV1 = "omnidex.application-requirements.v1"

type ApplicationRequirementKind string

const (
	ApplicationRequirementProduct ApplicationRequirementKind = "product"
	ApplicationRequirementFeature ApplicationRequirementKind = "feature"
)

type ApplicationRequirementInterpretationInput struct {
	UserRequest string `json:"user_request"`
}

type ApplicationRequirementItem struct {
	Kind        ApplicationRequirementKind `json:"kind"`
	SourceQuote string                     `json:"source_quote"`
}

type ApplicationRequirementInterpretation struct {
	Schema string                       `json:"schema"`
	Items  []ApplicationRequirementItem `json:"items"`
}

type ApplicationRequirementResolution struct {
	ProductQuote string
	Requirements []Requirement
}

func validateApplicationRequest(label, request string) error {
	if request == "" || request != strings.TrimSpace(request) {
		return fmt.Errorf("%s require one trimmed user request", label)
	}
	if len(request) > maxPortablePayloadBytes/2 {
		return fmt.Errorf("%s user request exceeds %d bytes", label, maxPortablePayloadBytes/2)
	}
	return nil
}

func validateApplicationProductQuote(label, quote string) error {
	if err := validateRequirementQuote(label, quote); err != nil {
		return err
	}
	if len(quote) > maxApplicationProductBytes {
		return fmt.Errorf("%s product quote exceeds %d bytes", label, maxApplicationProductBytes)
	}
	return nil
}

func (input ApplicationRequirementInterpretationInput) validate() error {
	return validateApplicationRequest("application requirements", input.UserRequest)
}

func ResolveApplicationRequirements(
	input ApplicationRequirementInterpretationInput,
	interpretation ApplicationRequirementInterpretation,
) (ApplicationRequirementResolution, error) {
	var zero ApplicationRequirementResolution
	if err := input.validate(); err != nil {
		return zero, err
	}
	if interpretation.Schema != ApplicationRequirementInterpretationSchemaV1 {
		return zero, fmt.Errorf(
			"application requirement schema must be %q",
			ApplicationRequirementInterpretationSchemaV1,
		)
	}
	if interpretation.Items == nil {
		return zero, fmt.Errorf("application requirements items must be an array")
	}
	if len(interpretation.Items) < 2 || len(interpretation.Items) > maxRequirementCount+1 {
		return zero, fmt.Errorf(
			"application requirements must contain one product and between 1 and %d features",
			maxRequirementCount,
		)
	}

	type groundedItem struct {
		item ApplicationRequirementItem
		span textSpan
	}
	grounded := make([]groundedItem, 0, len(interpretation.Items))
	seen := make(map[string]struct{}, len(interpretation.Items))
	productCount := 0
	featureCount := 0
	for index, item := range interpretation.Items {
		label := fmt.Sprintf("application requirement item %d", index)
		switch item.Kind {
		case ApplicationRequirementProduct:
			productCount++
			if err := validateApplicationProductQuote(label, item.SourceQuote); err != nil {
				return zero, err
			}
		case ApplicationRequirementFeature:
			featureCount++
			if err := validateRequirementQuote(label, item.SourceQuote); err != nil {
				return zero, err
			}
		default:
			return zero, fmt.Errorf("%s kind %q is unsupported", label, item.Kind)
		}
		if _, duplicate := seen[item.SourceQuote]; duplicate {
			return zero, fmt.Errorf("%s duplicates source quote %q", label, item.SourceQuote)
		}
		seen[item.SourceQuote] = struct{}{}
		span, err := uniqueTextSpan(input.UserRequest, item.SourceQuote)
		if err != nil {
			return zero, fmt.Errorf("%s source quote %q: %w", label, item.SourceQuote, err)
		}
		for _, prior := range grounded {
			if span.Overlaps(prior.span) {
				return zero, fmt.Errorf(
					"%s source quote %q overlaps %q",
					label, item.SourceQuote, prior.item.SourceQuote,
				)
			}
		}
		grounded = append(grounded, groundedItem{item: item, span: span})
	}
	if productCount != 1 {
		return zero, fmt.Errorf("application requirements require exactly one product quote")
	}
	if featureCount < 1 || featureCount > maxRequirementCount {
		return zero, fmt.Errorf(
			"application requirements require between 1 and %d feature quotes",
			maxRequirementCount,
		)
	}

	sort.SliceStable(grounded, func(left, right int) bool {
		return grounded[left].span.Start < grounded[right].span.Start
	})
	featureQuotes := make([]string, 0, featureCount)
	productQuote := ""
	for _, item := range grounded {
		switch item.item.Kind {
		case ApplicationRequirementProduct:
			productQuote = item.item.SourceQuote
		case ApplicationRequirementFeature:
			featureQuotes = append(featureQuotes, item.item.SourceQuote)
		}
	}
	graph, err := BuildRequirementGraph(input.UserRequest, featureQuotes)
	if err != nil {
		return zero, err
	}
	return ApplicationRequirementResolution{
		ProductQuote: productQuote,
		Requirements: graph.Requirements,
	}, nil
}
