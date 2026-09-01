package worker

import (
	"fmt"
	"strconv"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

type directCodingBrowserScreenQueryMethod struct {
	role         bool
	plural       bool
	asynchronous bool
	supported    bool
}

var directCodingBrowserScreenQueryMethods = map[string]directCodingBrowserScreenQueryMethod{
	"getByRole":              {role: true, supported: true},
	"findByRole":             {role: true, asynchronous: true, supported: true},
	"getAllByRole":           {role: true, plural: true, supported: true},
	"findAllByRole":          {role: true, plural: true, asynchronous: true, supported: true},
	"getByText":              {supported: true},
	"findByText":             {asynchronous: true, supported: true},
	"queryByRole":            {role: true},
	"queryAllByRole":         {role: true, plural: true},
	"queryByText":            {},
	"queryAllByText":         {plural: true},
	"getAllByText":           {plural: true},
	"findAllByText":          {plural: true, asynchronous: true},
	"getByTestId":            {},
	"findByTestId":           {asynchronous: true},
	"queryByTestId":          {},
	"getByLabelText":         {},
	"findByLabelText":        {asynchronous: true},
	"queryByLabelText":       {},
	"getByPlaceholderText":   {},
	"findByPlaceholderText":  {asynchronous: true},
	"queryByPlaceholderText": {},
	"debug":                  {},
}

func directCodingBrowserKnownScreenQueryMethod(name string) bool {
	_, exists := directCodingBrowserScreenQueryMethods[name]
	return exists
}

func (validator *directCodingBrowserAcceptanceQueryValidator) validateScreenMember(
	member *treesitter.Node,
	methodName string,
) error {
	method, known := directCodingBrowserScreenQueryMethods[methodName]
	if !known {
		return fmt.Errorf("browser acceptance screen query %s is unsupported", methodName)
	}
	if !method.supported {
		return fmt.Errorf(
			"browser acceptance screen query %s is unsupported by the grounded allowlist",
			methodName,
		)
	}
	if directCodingBrowserNodeHasChildKind(member, "optional_chain") {
		return fmt.Errorf("browser acceptance screen query %s cannot use optional chaining", methodName)
	}
	call := member.Parent()
	if call == nil || call.Kind() != "call_expression" ||
		!directCodingBrowserSameNode(call.ChildByFieldName("function"), member) {
		return fmt.Errorf("browser acceptance query %s must be called directly", methodName)
	}
	if err := validator.requireExecuted(call, true); err != nil {
		return err
	}
	var err error
	if method.role {
		err = validator.validateRoleQueryCall(call, methodName, method)
	} else {
		err = validator.validateTextQueryCall(call, methodName, method)
	}
	if err != nil {
		return err
	}
	return validator.validateScreenQueryConsumer(call, methodName, method)
}

func (validator *directCodingBrowserAcceptanceQueryValidator) validateRoleQueryCall(
	call *treesitter.Node,
	methodName string,
	method directCodingBrowserScreenQueryMethod,
) error {
	arguments := call.ChildByFieldName("arguments")
	if arguments == nil {
		return fmt.Errorf("browser acceptance role query %s has no arguments", methodName)
	}
	if method.plural && arguments.NamedChildCount() != 1 {
		return fmt.Errorf("browser acceptance plural role query %s forbids name filters and requires one role", methodName)
	}
	if !method.plural && (arguments.NamedChildCount() < 1 || arguments.NamedChildCount() > 2) {
		return fmt.Errorf("browser acceptance role query %s requires one role and one optional exact name", methodName)
	}
	role, err := validator.exactString(arguments.NamedChild(0))
	if err != nil || role == "" {
		return fmt.Errorf("browser acceptance role query %s requires an exact role string literal", methodName)
	}
	if role == "status" {
		return validator.validateOutputQueryCall(call, methodName, method, arguments)
	}
	matches := validator.controls(role, "", false)
	if len(matches) == 0 {
		return fmt.Errorf("browser acceptance public surface has no control with role %q", role)
	}
	if method.plural {
		index, selection, err := validator.literalPluralIndex(call, methodName, method.asynchronous)
		if err != nil {
			return err
		}
		if index >= len(matches) {
			return fmt.Errorf(
				"browser acceptance role query %s index %d is outside %d public matches",
				methodName, index, len(matches),
			)
		}
		validator.roleSelections[selection.Id()] = matches[index]
		return nil
	}
	name, hasName, err := validator.queryName(arguments)
	if err != nil {
		return fmt.Errorf("browser acceptance role query %s: %w", methodName, err)
	}
	if hasName {
		matches = validator.controls(role, name, true)
		if len(matches) == 0 {
			return fmt.Errorf(
				"browser acceptance public surface has no control with role %q and accessible name %q",
				role, name,
			)
		}
	}
	if len(matches) != 1 {
		return fmt.Errorf(
			"browser acceptance singular role query %s matches %d public controls; use one exact accessible name or an indexed all-query",
			methodName, len(matches),
		)
	}
	if method.asynchronous && !directCodingBrowserCallIsAwaited(call) {
		return fmt.Errorf("browser acceptance role query %s must be explicitly awaited", methodName)
	}
	selection := call
	if method.asynchronous {
		selection = directCodingBrowserAwaitExpression(call)
	}
	validator.roleSelections[selection.Id()] = matches[0]
	return nil
}

func (validator *directCodingBrowserAcceptanceQueryValidator) validateTextQueryCall(
	call *treesitter.Node,
	methodName string,
	method directCodingBrowserScreenQueryMethod,
) error {
	arguments := call.ChildByFieldName("arguments")
	if arguments == nil || arguments.NamedChildCount() != 1 {
		return fmt.Errorf("browser acceptance outcome query %s requires one exact text literal", methodName)
	}
	text, err := validator.exactString(arguments.NamedChild(0))
	if err != nil || text == "" {
		return fmt.Errorf("browser acceptance outcome query %s requires one non-empty exact text literal", methodName)
	}
	if method.asynchronous && !directCodingBrowserCallIsAwaited(call) {
		return fmt.Errorf("browser acceptance outcome query %s must be explicitly awaited", methodName)
	}
	return nil
}

func (validator *directCodingBrowserAcceptanceQueryValidator) queryName(
	arguments *treesitter.Node,
) (string, bool, error) {
	if arguments.NamedChildCount() == 1 {
		return "", false, nil
	}
	options := arguments.NamedChild(1)
	if options == nil || options.Kind() != "object" || options.NamedChildCount() != 1 ||
		options.NamedChild(0).Kind() != "pair" {
		return "", false, fmt.Errorf("selector options may contain only one exact name literal")
	}
	pair := options.NamedChild(0)
	key := pair.ChildByFieldName("key")
	value := pair.ChildByFieldName("value")
	if key == nil || value == nil || validator.text(key) != "name" {
		return "", false, fmt.Errorf("selector options may contain only one exact name literal")
	}
	name, err := validator.exactString(value)
	if err != nil || name == "" {
		return "", false, fmt.Errorf("accessible name must be one non-empty exact string literal")
	}
	if normalizeDirectCodingBrowserPublicLiteral(name) != name {
		return "", false, fmt.Errorf("accessible name literal is not in canonical public-text form")
	}
	return name, true, nil
}

func (validator *directCodingBrowserAcceptanceQueryValidator) controls(
	role string,
	name string,
	matchName bool,
) []directCodingBrowserPublicControl {
	matches := make([]directCodingBrowserPublicControl, 0)
	for _, control := range validator.surface.Controls {
		if control.Role == role && (!matchName || control.AccessibleName == name) {
			matches = append(matches, control)
		}
	}
	return matches
}

func (validator *directCodingBrowserAcceptanceQueryValidator) literalPluralIndex(
	call *treesitter.Node,
	methodName string,
	asynchronous bool,
) (int, *treesitter.Node, error) {
	selectionObject := call
	if asynchronous {
		parent := call.Parent()
		if parent == nil || parent.Kind() != "await_expression" {
			return 0, nil, fmt.Errorf("browser acceptance plural role query %s must be explicitly awaited before indexing", methodName)
		}
		selectionObject = parent
		for selectionObject.Parent() != nil && selectionObject.Parent().Kind() == "parenthesized_expression" {
			selectionObject = selectionObject.Parent()
		}
	}
	selection := selectionObject.Parent()
	if selection == nil || selection.Kind() != "subscript_expression" ||
		!directCodingBrowserSameNode(selection.ChildByFieldName("object"), selectionObject) {
		return 0, nil, fmt.Errorf("browser acceptance plural role query %s requires an exact literal index", methodName)
	}
	indexNode := selection.ChildByFieldName("index")
	if indexNode == nil || indexNode.Kind() != "number" {
		return 0, nil, fmt.Errorf("browser acceptance role query %s requires a literal zero-based index", methodName)
	}
	raw := validator.text(indexNode)
	index, err := strconv.Atoi(raw)
	if err != nil || index < 0 || strconv.Itoa(index) != raw {
		return 0, nil, fmt.Errorf("browser acceptance role query %s has a non-canonical literal index", methodName)
	}
	return index, selection, nil
}

func directCodingBrowserCallIsAwaited(call *treesitter.Node) bool {
	return directCodingBrowserAwaitExpression(call) != nil
}

func directCodingBrowserAwaitExpression(call *treesitter.Node) *treesitter.Node {
	current := call
	for current.Parent() != nil && current.Parent().Kind() == "parenthesized_expression" {
		current = current.Parent()
	}
	parent := current.Parent()
	if parent != nil && parent.Kind() == "await_expression" {
		return parent
	}
	return nil
}

func directCodingBrowserNodeHasChildKind(node *treesitter.Node, kind string) bool {
	for index := uint(0); index < node.ChildCount(); index++ {
		if child := node.Child(index); child != nil && child.Kind() == kind {
			return true
		}
	}
	return false
}
