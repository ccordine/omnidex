package assemblyline

import (
	"strconv"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

type acceptanceObservationCandidate struct {
	node      *treesitter.Node
	operation string
	residual  bool
	untrusted bool
}

func soleAcceptanceFunction(root *treesitter.Node) (*treesitter.Node, error) {
	if root == nil || root.NamedChildCount() != 1 {
		return nil, errAcceptanceSingleDeclaration
	}
	declaration := root.NamedChild(0)
	if declaration == nil || declaration.Kind() != "function_declaration" {
		return nil, errAcceptanceSingleFunction
	}
	return declaration, nil
}

var errAcceptanceSingleDeclaration = newAcceptanceInventoryError(
	"acceptance observation source requires exactly one declaration",
)
var errAcceptanceSingleFunction = newAcceptanceInventoryError(
	"acceptance observation source requires exactly one function declaration",
)

type acceptanceInventoryError string

func newAcceptanceInventoryError(value string) error { return acceptanceInventoryError(value) }
func (value acceptanceInventoryError) Error() string { return string(value) }

func newAcceptanceObservationStatement(
	id string,
	node *treesitter.Node,
	source []byte,
) AcceptanceObservationStatement {
	structure, operators := acceptanceNodeStructure(node, source)
	return AcceptanceObservationStatement{
		ID: id, StatementKind: node.Kind(), Structure: structure, Operators: operators,
	}
}

func collectAcceptanceCallCandidates(
	node *treesitter.Node,
	source []byte,
) []acceptanceObservationCandidate {
	result := []acceptanceObservationCandidate{}
	var visit func(*treesitter.Node)
	visit = func(current *treesitter.Node) {
		if current == nil || current.Kind() == "comment" {
			return
		}
		if current.Kind() == "call_expression" {
			if operation := acceptanceCallOperation(current, source); operation != "" {
				result = append(result, acceptanceObservationCandidate{
					node: current, operation: operation,
				})
			}
		}
		for index := uint(0); index < current.NamedChildCount(); index++ {
			visit(current.NamedChild(index))
		}
	}
	visit(node)
	return result
}

func newAcceptanceObservationSite(
	siteID string,
	assertionID string,
	statementID string,
	statementKind string,
	candidate acceptanceObservationCandidate,
	source []byte,
) AcceptanceObservationSite {
	structure, operators := acceptanceNodeStructure(candidate.node, source)
	if candidate.residual {
		structure, operators = acceptanceResidualStructure(candidate.node, source)
	}
	literals := acceptanceCandidateLiterals(candidate, source)
	operations := []string{candidate.operation}
	if !candidate.residual && !acceptancePlatformOperation(candidate.operation) {
		semantics := collectAcceptanceCandidateSemantics(candidate.node, candidate.operation, source)
		literals = semantics.literals
		operations = append(operations, semantics.operations...)
		if !semantics.trusted {
			operations = appendAcceptanceOperation(operations, "untrusted_call")
		}
	}
	if candidate.untrusted {
		operations = appendAcceptanceOperation(operations, "untrusted_call")
	}
	return AcceptanceObservationSite{
		ID: siteID, AssertionID: assertionID, StatementID: statementID,
		StatementKind: statementKind, Structure: structure,
		Operations: operations, Operators: operators, Literals: literals,
	}
}

func acceptanceNodeStructure(node *treesitter.Node, source []byte) ([]string, []string) {
	structure := []string{}
	operators := []string{}
	var visit func(*treesitter.Node)
	visit = func(current *treesitter.Node) {
		if current == nil || current.Kind() == "comment" {
			return
		}
		if !acceptanceIdentifierKind(current.Kind()) {
			structure = append(structure, current.Kind())
		}
		if operator := acceptanceOperator(current, source); operator != "" {
			operators = append(operators, operator)
		}
		for index := uint(0); index < current.NamedChildCount(); index++ {
			visit(current.NamedChild(index))
		}
	}
	visit(node)
	return structure, operators
}

func acceptanceCandidateLiterals(
	candidate acceptanceObservationCandidate,
	source []byte,
) []AcceptanceObservationLiteral {
	literals := []AcceptanceObservationLiteral{}
	if acceptancePlatformOperation(candidate.operation) {
		return literals
	}
	if candidate.residual {
		walkAcceptanceResidual(candidate.node, source, func(current *treesitter.Node) {
			if literal, ok := acceptanceLiteral(current, source); ok {
				literals = append(literals, literal)
			}
		})
		return literals
	}
	root := candidate.node
	if root.Kind() == "call_expression" {
		root = root.ChildByFieldName("arguments")
	}
	var visit func(*treesitter.Node)
	visit = func(current *treesitter.Node) {
		if current == nil || current.Kind() == "comment" {
			return
		}
		if current.Kind() == "call_expression" {
			return
		}
		if literal, ok := acceptanceLiteral(current, source); ok {
			literals = append(literals, literal)
			return
		}
		for index := uint(0); index < current.NamedChildCount(); index++ {
			visit(current.NamedChild(index))
		}
	}
	visit(root)
	return literals
}

func acceptanceIdentifierKind(kind string) bool {
	return strings.Contains(kind, "identifier") || kind == "property_identifier" ||
		kind == "shorthand_property_identifier_pattern" || kind == "jsx_namespace_name"
}

func acceptanceLiteral(node *treesitter.Node, source []byte) (AcceptanceObservationLiteral, bool) {
	raw := node.Utf8Text(source)
	switch node.Kind() {
	case "string":
		return AcceptanceObservationLiteral{Kind: "string", Value: decodeAcceptanceString(raw)}, true
	case "number":
		return AcceptanceObservationLiteral{Kind: "number", Value: raw}, true
	case "true", "false":
		return AcceptanceObservationLiteral{Kind: "boolean", Value: raw}, true
	case "null":
		return AcceptanceObservationLiteral{Kind: "null", Value: "null"}, true
	case "regex":
		return AcceptanceObservationLiteral{Kind: "regular_expression", Value: raw}, true
	case "template_string":
		for index := uint(0); index < node.NamedChildCount(); index++ {
			if node.NamedChild(index).Kind() == "template_substitution" {
				return AcceptanceObservationLiteral{}, false
			}
		}
		if len(raw) >= 2 {
			raw = raw[1 : len(raw)-1]
		}
		return AcceptanceObservationLiteral{Kind: "template_string", Value: raw}, true
	default:
		return AcceptanceObservationLiteral{}, false
	}
}

func decodeAcceptanceString(raw string) string {
	if len(raw) < 2 {
		return raw
	}
	if raw[0] == '"' {
		if decoded, err := strconv.Unquote(raw); err == nil {
			return decoded
		}
	}
	inner := raw[1 : len(raw)-1]
	replacer := strings.NewReplacer(`\\`, `\`, `\'`, `'`, `\"`, `"`, `\n`, "\n", `\r`, "\r", `\t`, "\t")
	return replacer.Replace(inner)
}
