package worker

import (
	"fmt"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func (flow *phpHTMLRepresentationFlow) consumeHTMLCall(
	call *treesitter.Node,
) (string, error) {
	switch {
	case phpExactScopedCall(flow.source, call, "RuntimeHtml", "escape"):
		return flow.consumeEscapedField(call)
	case phpExactScopedCall(flow.source, call, "RuntimeHtml", "state"):
		arguments := phpCallArguments(call.ChildByFieldName("arguments"))
		field, ok := phpExactResultField(flow.source, firstPHPArgument(arguments), "$result")
		if len(arguments) != 1 || !ok || field != "state" {
			return "", fmt.Errorf("RuntimeHtml::state requires the exact TaskResult state field")
		}
		flow.fields++
		return "omnidex-result", nil
	case phpExactScopedCall(flow.source, call, "RuntimeHtml", "formOpen"):
		arguments := phpCallArguments(call.ChildByFieldName("arguments"))
		if len(arguments) != 1 {
			return "", fmt.Errorf("RuntimeHtml::formOpen requires one registered route")
		}
		if err := flow.consumeRouteCall(arguments[0]); err != nil {
			return "", err
		}
		return `<form action="/omnidex-route" method="post">`, nil
	case phpExactScopedCall(flow.source, call, "RuntimeHtml", "formClose"):
		if len(phpCallArguments(call.ChildByFieldName("arguments"))) != 0 {
			return "", fmt.Errorf("RuntimeHtml::formClose does not accept arguments")
		}
		return "</form>", nil
	case phpExactScopedCall(flow.source, call, "RuntimeHtml", "link"):
		arguments := phpCallArguments(call.ChildByFieldName("arguments"))
		if len(arguments) != 2 {
			return "", fmt.Errorf("RuntimeHtml::link requires one registered route and label")
		}
		if err := flow.consumeRouteCall(arguments[0]); err != nil {
			return "", err
		}
		label, err := phpHTMLSingleQuotedLiteral(flow.source, arguments[1])
		if err != nil || strings.TrimSpace(label) == "" {
			return "", fmt.Errorf("RuntimeHtml::link label is invalid")
		}
		return `<a href="/omnidex-route">` + label + `</a>`, nil
	default:
		return "", fmt.Errorf("PHP HTML renderer body calls an unsupported renderer method")
	}
}

func (flow *phpHTMLRepresentationFlow) consumeEscapedField(
	call *treesitter.Node,
) (string, error) {
	arguments := phpCallArguments(call.ChildByFieldName("arguments"))
	if len(arguments) != 1 {
		return "", fmt.Errorf("RuntimeHtml::escape requires one exact result or record field")
	}
	if field, ok := phpExactResultField(flow.source, arguments[0], "$result"); ok {
		if field != "output" && field != "error" {
			return "", fmt.Errorf("RuntimeHtml::escape received an incompatible TaskResult field")
		}
		flow.fields++
		return "omnidex-result", nil
	}
	if err := flow.consumeRecordField(arguments[0]); err != nil {
		return "", fmt.Errorf("RuntimeHtml::escape: %w", err)
	}
	flow.fields++
	return "omnidex-record-field", nil
}

func (flow *phpHTMLRepresentationFlow) consumeRecordField(node *treesitter.Node) error {
	node = phpAcceptanceUnwrapParentheses(node)
	if !phpExactScopedCall(flow.source, node, "RuntimeHtml", "field") {
		return fmt.Errorf("record values must use RuntimeHtml::field")
	}
	arguments := phpCallArguments(node.ChildByFieldName("arguments"))
	if len(arguments) != 2 || arguments[0] == nil || arguments[0].Kind() != "variable_name" {
		return fmt.Errorf("RuntimeHtml::field requires one traversed record and field key")
	}
	if _, exists := flow.records[phpNodeText(flow.source, arguments[0])]; !exists {
		return fmt.Errorf("RuntimeHtml::field received an unbound record")
	}
	key, err := phpHTMLSingleQuotedLiteral(flow.source, arguments[1])
	if err != nil || strings.TrimSpace(key) == "" {
		return fmt.Errorf("RuntimeHtml::field key is invalid")
	}
	return nil
}

func (flow *phpHTMLRepresentationFlow) consumeRouteCall(node *treesitter.Node) error {
	node = phpAcceptanceUnwrapParentheses(node)
	if node == nil || node.Kind() != "function_call_expression" {
		return fmt.Errorf("HTML interaction requires one registered route function")
	}
	function := node.ChildByFieldName("function")
	if function == nil || function.Kind() != "name" ||
		!phpHTMLRouteFunction.MatchString(phpNodeText(flow.source, function)) {
		return fmt.Errorf("HTML interaction route function is invalid")
	}
	for _, argument := range phpCallArguments(node.ChildByFieldName("arguments")) {
		if err := flow.consumeRecordField(argument); err != nil {
			return fmt.Errorf("HTML route parameter: %w", err)
		}
	}
	return nil
}
