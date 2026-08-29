package contextcompiler

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

// ResolveRetrievalDirective binds fixed provider availability to an immutable
// code-owned directive. It performs no inference: when search is available,
// Compile passes the exact authoritative instruction as the sole query.
func ResolveRetrievalDirective(
	ctx context.Context,
	exactInstruction string,
	scope assemblyline.ContextScope,
	availability SearchAvailability,
) (RetrievalDirective, error) {
	empty := RetrievalDirective{}
	if ctx == nil {
		return empty, fmt.Errorf("context retrieval resolution requires a context")
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	if err := validateRetrievalAuthority(exactInstruction, scope); err != nil {
		return empty, err
	}
	if err := availability.Validate(); err != nil {
		return empty, err
	}
	return RetrievalDirective{Availability: availability}, nil
}

func validateRetrievalAuthority(
	exactInstruction string,
	scope assemblyline.ContextScope,
) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(exactInstruction) == "" {
		return fmt.Errorf("context retrieval requires one non-blank exact instruction")
	}
	if len(exactInstruction) > model.MaxFreeFormTurnBytes {
		return fmt.Errorf(
			"context retrieval exact instruction exceeds %d bytes",
			model.MaxFreeFormTurnBytes,
		)
	}
	if !utf8.ValidString(exactInstruction) {
		return fmt.Errorf("context retrieval exact instruction is not valid UTF-8")
	}
	if strings.ContainsRune(exactInstruction, '\x00') {
		return fmt.Errorf("context retrieval exact instruction contains NUL")
	}
	return nil
}

func retrievalQueries(
	exactInstruction string,
	scope assemblyline.ContextScope,
	directive RetrievalDirective,
) ([]string, error) {
	if err := validateRetrievalAuthority(exactInstruction, scope); err != nil {
		return nil, err
	}
	if err := directive.Availability.Validate(); err != nil {
		return nil, err
	}
	if directive.Availability == SearchUnavailable {
		return []string{}, nil
	}
	return []string{exactInstruction}, nil
}
