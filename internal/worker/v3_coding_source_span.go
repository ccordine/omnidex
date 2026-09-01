package worker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
)

// directCodingSourceNodeFailure is deterministic validator evidence that one
// exact node in a code-assembled declaration is the failed semantic leaf. It
// remains declaration-relative until the source adapter proves that the node
// lies wholly inside the model-returned body.
type directCodingSourceNodeFailure struct {
	startByte             int
	endByte               int
	question              string
	identifierReplacement bool
	replacements          []assemblyline.OpaqueModelChoice
	cause                 error
}

func (failure *directCodingSourceNodeFailure) Error() string {
	if failure == nil || failure.cause == nil {
		return "source node failure is nil"
	}
	return failure.cause.Error()
}

func directCodingIdentifierNodeError(
	node *treesitter.Node,
	question string,
	replacements []assemblyline.OpaqueModelChoice,
	cause error,
) error {
	if node == nil || cause == nil {
		return cause
	}
	if len(replacements) == 0 {
		return fmt.Errorf(
			"%w: deterministic scope found no authorized replacement for the exact failed token",
			cause,
		)
	}
	return &directCodingSourceNodeFailure{
		startByte:             int(node.StartByte()),
		endByte:               int(node.EndByte()),
		question:              question,
		identifierReplacement: true,
		replacements:          append([]assemblyline.OpaqueModelChoice(nil), replacements...),
		cause:                 cause,
	}
}

func (failure *directCodingSourceNodeFailure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.cause
}

func directCodingSourceNodeError(
	node *treesitter.Node,
	question string,
	cause error,
) error {
	if node == nil || cause == nil {
		return cause
	}
	return &directCodingSourceNodeFailure{
		startByte: int(node.StartByte()),
		endByte:   int(node.EndByte()),
		question:  question,
		cause:     cause,
	}
}

// directCodingSourceBodyError maps a validator-owned declaration node to the
// exact returned-body bytes. If code cannot prove the declaration composition
// or the node crosses code-owned structure, the original failure stays loud
// and unlocated instead of authorizing inference over a broader region.
func directCodingSourceBodyError(
	input assemblyline.FragmentGenerationInput,
	body string,
	declaration string,
	validationErr error,
) error {
	var located *directCodingSourceNodeFailure
	if !errors.As(validationErr, &located) {
		return validationErr
	}
	prefix := strings.TrimSpace(input.Signature) + " {\n"
	if declaration != prefix+body+"\n}" {
		return fmt.Errorf(
			"source validator could not prove the code-owned declaration projection: %w",
			validationErr,
		)
	}
	bodyStart := len(prefix)
	bodyEnd := bodyStart + len(body)
	if located.startByte < bodyStart || located.endByte > bodyEnd ||
		located.startByte >= located.endByte {
		return validationErr
	}
	var defect *assemblyline.SourceBodyDefect
	var err error
	if located.identifierReplacement {
		defect, err = assemblyline.NewSourceBodyIdentifierDefect(
			body,
			located.startByte-bodyStart,
			located.endByte-bodyStart,
			located.question,
			validationErr,
			located.replacements,
		)
	} else {
		defect, err = assemblyline.NewSourceBodyDefect(
			body,
			located.startByte-bodyStart,
			located.endByte-bodyStart,
			located.question,
			validationErr,
		)
	}
	if err != nil {
		return fmt.Errorf("map exact source node to implementation body: %w", err)
	}
	return defect
}
