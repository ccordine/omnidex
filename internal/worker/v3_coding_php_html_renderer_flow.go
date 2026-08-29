package worker

import (
	"fmt"
	"regexp"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

var phpHTMLRouteFunction = regexp.MustCompile(`^routeFeature[0-9]{3}$`)

type phpHTMLRepresentationFlow struct {
	source  []byte
	values  map[string]string
	records map[string]struct{}
	fields  int
}

func newPHPHTMLRepresentationFlow(source []byte) *phpHTMLRepresentationFlow {
	return &phpHTMLRepresentationFlow{
		source: source, values: make(map[string]string), records: make(map[string]struct{}),
	}
}

func (flow *phpHTMLRepresentationFlow) consumeBody(body *treesitter.Node) (string, error) {
	count := body.NamedChildCount()
	for index := uint(0); index+1 < count; index++ {
		if err := flow.consumeStatement(body.NamedChild(index)); err != nil {
			return "", err
		}
	}
	last := body.NamedChild(count - 1)
	if last == nil || last.Kind() != "return_statement" || last.NamedChildCount() != 1 {
		return "", fmt.Errorf("PHP HTML renderer must end with one direct document return")
	}
	call := phpAcceptanceUnwrapParentheses(last.NamedChild(0))
	if !phpExactScopedCall(flow.source, call, "RuntimeHtml", "document") {
		return "", fmt.Errorf("PHP HTML renderer must directly return RuntimeHtml::document")
	}
	arguments := phpCallArguments(call.ChildByFieldName("arguments"))
	if len(arguments) != 2 {
		return "", fmt.Errorf("RuntimeHtml::document requires one title and one body")
	}
	title, err := phpHTMLSingleQuotedLiteral(flow.source, arguments[0])
	if err != nil || strings.TrimSpace(title) == "" {
		return "", fmt.Errorf("PHP HTML renderer title must be one non-empty single-quoted literal")
	}
	return flow.consumeExpression(arguments[1])
}

func (flow *phpHTMLRepresentationFlow) consumeStatement(statement *treesitter.Node) error {
	if statement == nil {
		return fmt.Errorf("PHP HTML renderer contains an empty statement")
	}
	switch statement.Kind() {
	case "expression_statement":
		if statement.NamedChildCount() != 1 {
			return fmt.Errorf("PHP HTML renderer expression statement is not bounded")
		}
		return flow.consumeAssignment(statement.NamedChild(0))
	case "foreach_statement":
		return flow.consumeForeach(statement)
	default:
		return fmt.Errorf("PHP HTML renderer contains unsupported %s statement", statement.Kind())
	}
}

func (flow *phpHTMLRepresentationFlow) consumeAssignment(expression *treesitter.Node) error {
	expression = phpAcceptanceUnwrapParentheses(expression)
	if expression == nil ||
		(expression.Kind() != "assignment_expression" && expression.Kind() != "augmented_assignment_expression") {
		return fmt.Errorf("PHP HTML renderer permits only local HTML assignments before its return")
	}
	left := expression.ChildByFieldName("left")
	if left == nil || left.Kind() != "variable_name" {
		return fmt.Errorf("PHP HTML renderer assignment requires one local variable")
	}
	name := phpNodeText(flow.source, left)
	if name == "$result" {
		return fmt.Errorf("PHP HTML renderer cannot mutate its TaskResult")
	}
	if _, record := flow.records[name]; record {
		return fmt.Errorf("PHP HTML renderer cannot mutate a traversed record")
	}
	value, err := flow.consumeExpression(expression.ChildByFieldName("right"))
	if err != nil {
		return err
	}
	if expression.Kind() == "assignment_expression" {
		flow.values[name] = value
		return nil
	}
	if phpNodeText(flow.source, expression.ChildByFieldName("operator")) != ".=" {
		return fmt.Errorf("PHP HTML renderer permits only string append assignment")
	}
	current, exists := flow.values[name]
	if !exists {
		return fmt.Errorf("PHP HTML renderer appends to undeclared local %s", name)
	}
	flow.values[name] = current + value
	return nil
}

func (flow *phpHTMLRepresentationFlow) consumeForeach(statement *treesitter.Node) error {
	body := statement.ChildByFieldName("body")
	if body == nil || body.Kind() != "compound_statement" || statement.NamedChildCount() != 3 {
		return fmt.Errorf("PHP HTML renderer foreach requires one bounded compound body")
	}
	iterable, record := statement.NamedChild(0), statement.NamedChild(1)
	if record == nil || record.Kind() != "variable_name" {
		return fmt.Errorf("PHP HTML renderer foreach requires one record value")
	}
	name := phpNodeText(flow.source, record)
	if name == "$result" {
		return fmt.Errorf("PHP HTML renderer foreach record identity is invalid")
	}
	if err := flow.consumeRecordsCall(iterable); err != nil {
		return err
	}
	if _, exists := flow.records[name]; exists {
		return fmt.Errorf("PHP HTML renderer repeats record variable %s", name)
	}
	flow.records[name] = struct{}{}
	defer delete(flow.records, name)
	for index := uint(0); index < body.NamedChildCount(); index++ {
		if err := flow.consumeStatement(body.NamedChild(index)); err != nil {
			return err
		}
	}
	return nil
}

func (flow *phpHTMLRepresentationFlow) consumeRecordsCall(node *treesitter.Node) error {
	node = phpAcceptanceUnwrapParentheses(node)
	if !phpExactScopedCall(flow.source, node, "RuntimeHtml", "records") {
		return fmt.Errorf("PHP HTML renderer may traverse only RuntimeHtml::records")
	}
	arguments := phpCallArguments(node.ChildByFieldName("arguments"))
	field, ok := phpExactResultField(flow.source, firstPHPArgument(arguments), "$result")
	if len(arguments) != 2 || !ok || field != "state" {
		return fmt.Errorf("RuntimeHtml::records requires TaskResult state and one collection key")
	}
	key, err := phpHTMLSingleQuotedLiteral(flow.source, arguments[1])
	if err != nil || strings.TrimSpace(key) == "" {
		return fmt.Errorf("RuntimeHtml::records collection key is invalid")
	}
	return nil
}

func (flow *phpHTMLRepresentationFlow) consumeExpression(node *treesitter.Node) (string, error) {
	node = phpAcceptanceUnwrapParentheses(node)
	if node == nil {
		return "", fmt.Errorf("PHP HTML renderer contains an empty expression")
	}
	switch node.Kind() {
	case "string":
		return phpHTMLSingleQuotedLiteral(flow.source, node)
	case "binary_expression":
		if phpNodeText(flow.source, node.ChildByFieldName("operator")) != "." {
			return "", fmt.Errorf("PHP HTML renderer body permits only string concatenation")
		}
		left, err := flow.consumeExpression(node.ChildByFieldName("left"))
		if err != nil {
			return "", err
		}
		right, err := flow.consumeExpression(node.ChildByFieldName("right"))
		return left + right, err
	case "variable_name":
		value, exists := flow.values[phpNodeText(flow.source, node)]
		if !exists {
			return "", fmt.Errorf("PHP HTML renderer emits an undeclared local value")
		}
		return value, nil
	case "scoped_call_expression":
		return flow.consumeHTMLCall(node)
	default:
		return "", fmt.Errorf("PHP HTML renderer body contains unsupported %s expression", node.Kind())
	}
}

func firstPHPArgument(arguments []*treesitter.Node) *treesitter.Node {
	if len(arguments) == 0 {
		return nil
	}
	return arguments[0]
}
