package assemblyline

import (
	"fmt"
)

// BuildRequirementResidual removes only code-accepted exact spans. It preserves
// byte offsets and line breaks so every remaining model quote can be validated
// against both the residual and the original authority.
func BuildRequirementResidual(source string, coveredQuotes []string) (string, error) {
	if source == "" {
		return "", fmt.Errorf("requirement residual requires source text")
	}
	if err := validateQuoteCollection("requirement residual covered", coveredQuotes); err != nil {
		return "", err
	}
	spans := make([]textSpan, 0, len(coveredQuotes))
	for index, quote := range coveredQuotes {
		span, err := uniqueTextSpan(source, quote)
		if err != nil {
			return "", fmt.Errorf("covered quote %d %q: %w", index, quote, err)
		}
		for _, prior := range spans {
			if span.Overlaps(prior) {
				return "", fmt.Errorf("covered quote %d %q overlaps another covered span", index, quote)
			}
		}
		spans = append(spans, span)
	}

	residual := []byte(source)
	for _, span := range spans {
		for index := span.Start; index < span.End; index++ {
			if residual[index] != '\n' && residual[index] != '\r' {
				residual[index] = ' '
			}
		}
	}
	return string(residual), nil
}
