package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// TypeScriptFunctionProjection binds one exact source declaration to its byte
// span in the complete untrusted model response. Source is never reconstructed,
// normalized, or summarized: it is the exact substring selected by StartByte
// and EndByte.
type TypeScriptFunctionProjection struct {
	Source         string
	RawSHA256      string
	SourceSHA256   string
	StartByte      int
	EndByte        int
	RawBytes       int
	SourceBytes    int
	DiscardedBytes int
}

type typeScriptFunctionNode struct {
	name       string
	startByte  int
	endByte    int
	exportWrap bool
}

type typeScriptResponseSegment struct {
	startByte int
	endByte   int
	fenced    bool
}

// ProjectTypeScriptFunctionModelResponse treats the complete final response as
// untrusted text and selects the unique required TypeScript function node. The
// parser, policy validator, compiler, and acceptance checks remain downstream;
// this boundary owns only exact artifact selection and expanded-authority
// rejection.
func ProjectTypeScriptFunctionModelResponse(
	contract TypeScriptFunctionContract,
	raw string,
) (TypeScriptFunctionProjection, error) {
	var zero TypeScriptFunctionProjection
	if raw == "" || strings.TrimSpace(raw) == "" {
		return zero, fmt.Errorf("TypeScript model response is empty")
	}
	if !utf8.ValidString(raw) || strings.ContainsRune(raw, 0) {
		return zero, fmt.Errorf("TypeScript model response must be valid UTF-8 without NUL bytes")
	}
	signature := strings.TrimSpace(contract.Signature)
	if signature == "" || strings.ContainsAny(signature, "\r\n") {
		return zero, fmt.Errorf("TypeScript function contract requires one single-line signature")
	}
	expected, closeExpected, err := parseSingleTypeScriptFunction(
		signature+" {}", contract.TSX, false, SourceFunctionPolicy{},
	)
	if err != nil {
		return zero, fmt.Errorf("invalid code-owned TypeScript signature: %w", err)
	}
	defer closeExpected()

	segments := typeScriptResponseSegments(raw, contract.TSX)
	functions := make([]typeScriptFunctionNode, 0, 2)
	for _, segment := range segments {
		segmentFunctions, err := parseTypeScriptResponseFunctions(
			raw[segment.startByte:segment.endByte], segment.startByte, contract.TSX,
		)
		if err != nil {
			return zero, err
		}
		functions = append(functions, segmentFunctions...)
	}
	matches := make([]typeScriptFunctionNode, 0, 1)
	for _, function := range functions {
		if function.name == expected.name {
			matches = append(matches, function)
		}
	}
	if len(matches) == 0 {
		return zero, fmt.Errorf(
			"TypeScript model response contains no required function declaration %q",
			expected.name,
		)
	}
	if len(matches) > 1 {
		return zero, fmt.Errorf(
			"TypeScript model response contains %d ambiguous declarations named %q",
			len(matches), expected.name,
		)
	}
	selected := matches[0]
	projection, err := newTypeScriptFunctionProjection(raw, selected)
	if err != nil {
		return zero, err
	}
	if selected.exportWrap {
		return projection, fmt.Errorf(
			"TypeScript model response must be one raw function declaration; required declaration is wrapped in extra export authority",
		)
	}
	if len(functions) != 1 {
		return projection, fmt.Errorf(
			"TypeScript model response contains %d function declarations; exactly one required declaration is allowed",
			len(functions),
		)
	}
	for _, segment := range segments {
		kind, err := extraTypeScriptExecutableInSegment(
			raw[segment.startByte:segment.endByte], segment.startByte,
			selected.startByte, selected.endByte, contract.TSX,
		)
		if err != nil {
			return projection, err
		}
		if kind != "" {
			return projection, fmt.Errorf(
				"TypeScript model response contains extra executable node %s outside the required declaration",
				kind,
			)
		}
	}
	return projection, nil
}

func newTypeScriptFunctionProjection(
	raw string,
	selected typeScriptFunctionNode,
) (TypeScriptFunctionProjection, error) {
	if selected.startByte < 0 || selected.endByte <= selected.startByte || selected.endByte > len(raw) {
		return TypeScriptFunctionProjection{}, fmt.Errorf(
			"TypeScript response projector produced an invalid source span",
		)
	}
	source := raw[selected.startByte:selected.endByte]
	return TypeScriptFunctionProjection{
		Source: source, RawSHA256: typeScriptProjectionSHA256(raw),
		SourceSHA256: typeScriptProjectionSHA256(source),
		StartByte:    selected.startByte, EndByte: selected.endByte,
		RawBytes: len(raw), SourceBytes: len(source),
		DiscardedBytes: len(raw) - len(source),
	}, nil
}

func parseTypeScriptResponseFunctions(
	source string,
	baseOffset int,
	tsx bool,
) ([]typeScriptFunctionNode, error) {
	parser, tree, err := parseTypeScriptResponseTree(source, tsx)
	if err != nil {
		return nil, err
	}
	defer parser.Close()
	defer tree.Close()
	local := collectTypeScriptResponseFunctions(tree.RootNode(), []byte(source), false, nil)
	for index := range local {
		local[index].startByte += baseOffset
		local[index].endByte += baseOffset
	}
	return local, nil
}

func extraTypeScriptExecutableInSegment(
	source string,
	baseOffset int,
	selectedStart int,
	selectedEnd int,
	tsx bool,
) (string, error) {
	parser, tree, err := parseTypeScriptResponseTree(source, tsx)
	if err != nil {
		return "", err
	}
	defer parser.Close()
	defer tree.Close()
	return firstExtraTypeScriptExecutableNode(
		tree.RootNode(), selectedStart-baseOffset, selectedEnd-baseOffset,
	), nil
}
