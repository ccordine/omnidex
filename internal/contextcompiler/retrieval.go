package contextcompiler

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// ResolveRetrievalDirective answers the one semantic retrieval-concept
// question only when code has proved that term-directed acquisition may add
// authority. The returned directive is immutable code-owned input that can be
// reused across multiple viewpoint-specific compilations of the same exact
// instruction.
func ResolveRetrievalDirective(
	ctx context.Context,
	exactInstruction string,
	availability SearchAvailability,
	station SearchTermsStation,
) (RetrievalDirective, int, error) {
	empty := RetrievalDirective{Concepts: []string{}}
	if ctx == nil {
		return empty, 0, fmt.Errorf("context retrieval resolution requires a context")
	}
	if err := ctx.Err(); err != nil {
		return empty, 0, err
	}
	input := assemblyline.ContextSearchTermsInput{ExactInstruction: exactInstruction}
	if _, err := assemblyline.NewContextSearchTermsJob(input); err != nil {
		return empty, 0, err
	}
	if err := availability.Validate(); err != nil {
		return empty, 0, err
	}
	if availability == SearchUnavailable {
		return empty, 0, nil
	}
	if station == nil {
		return empty, 0, fmt.Errorf(
			"context search terms remain unresolved but the station is unavailable",
		)
	}
	decision, receipt, err := station.Generate(ctx, input)
	if err != nil {
		return empty, 0, fmt.Errorf("context search terms: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return empty, 0, err
	}
	if err := validateReceipt("context search terms", receipt); err != nil {
		return empty, 0, err
	}
	if err := decision.ValidateFor(input); err != nil {
		return empty, 0, err
	}
	return RetrievalDirective{
		Concepts: canonicalRetrievalConcepts(decision.Terms),
	}, receipt.Calls, nil
}
