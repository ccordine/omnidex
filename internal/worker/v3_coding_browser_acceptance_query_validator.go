package worker

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

type directCodingBrowserAcceptanceQueryValidator struct {
	source                 []byte
	surface                directCodingBrowserPublicInteractionSurface
	rootFunction           *treesitter.Node
	roleSelections         map[uintptr]directCodingBrowserPublicControl
	outputSelections       map[uintptr]directCodingBrowserPublicOutput
	screenQuerySelections  map[uintptr]string
	outcomeAssertionStarts []uint
	outputAssertionStarts  []uint
	unprovenTextStarts     []uint
	finalFireEventEnd      uint
	nodes                  int
	executedAsserts        int
}

// This validates only mechanically provable public selectors and event shapes.
// Expected outcome text remains unknown until the generated verification runs.
func validateDirectCodingBrowserAcceptanceRoleQueries(
	source string,
	tsx bool,
	surface directCodingBrowserPublicInteractionSurface,
	resultRelation string,
) error {
	switch resultRelation {
	case assemblyline.ApplicationRequirementNoDerivedResult,
		assemblyline.ApplicationRequirementExplicitResultRelation:
	case assemblyline.ApplicationRequirementMissingResultRelation:
		return fmt.Errorf(
			"browser acceptance cannot validate non-retainable result relation %q",
			resultRelation,
		)
	default:
		return fmt.Errorf(
			"browser acceptance result relation %q is not registered", resultRelation,
		)
	}
	if source == "" || !utf8.ValidString(source) {
		return fmt.Errorf("browser acceptance query source is empty or invalid UTF-8")
	}
	if len(source) > directCodingBrowserPublicSurfaceMaxSourceBytes {
		return fmt.Errorf(
			"browser acceptance queries exceed %d source bytes",
			directCodingBrowserPublicSurfaceMaxSourceBytes,
		)
	}
	if _, err := renderDirectCodingBrowserPublicInteractionSurface(surface); err != nil {
		return fmt.Errorf("browser acceptance query surface: %w", err)
	}
	parser := treesitter.NewParser()
	defer parser.Close()
	language := typescript.LanguageTypescript()
	if tsx {
		language = typescript.LanguageTSX()
	}
	if err := parser.SetLanguage(treesitter.NewLanguage(language)); err != nil {
		return fmt.Errorf("configure browser acceptance query parser: %w", err)
	}
	tree := parser.Parse([]byte(source), nil)
	if tree == nil {
		return fmt.Errorf("browser acceptance query parser returned no tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		return fmt.Errorf("browser acceptance query source is not valid TypeScript")
	}
	if err := rejectDirectCodingBrowserAcceptanceAuthorityShadowing(root, []byte(source)); err != nil {
		return err
	}
	rootFunction, err := directCodingBrowserAcceptanceExecutionRoot(root)
	if err != nil {
		return err
	}
	validator := directCodingBrowserAcceptanceQueryValidator{
		source: sourceBytes(source), surface: surface, rootFunction: rootFunction,
		roleSelections:        make(map[uintptr]directCodingBrowserPublicControl),
		outputSelections:      make(map[uintptr]directCodingBrowserPublicOutput),
		screenQuerySelections: make(map[uintptr]string),
	}
	if err := validator.inspect(root); err != nil {
		return err
	}
	if err := validator.validateFlatExecutionBodies(); err != nil {
		return err
	}
	return validator.validateRequiredOutcomes(resultRelation)
}

func sourceBytes(source string) []byte {
	return []byte(source)
}

func (validator *directCodingBrowserAcceptanceQueryValidator) inspect(
	node *treesitter.Node,
) error {
	if node == nil {
		return nil
	}
	validator.nodes++
	if validator.nodes > directCodingBrowserPublicSurfaceMaxNodes {
		return fmt.Errorf(
			"browser acceptance queries exceed %d syntax nodes",
			directCodingBrowserPublicSurfaceMaxNodes,
		)
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if err := validator.inspect(node.NamedChild(index)); err != nil {
			return err
		}
	}
	switch node.Kind() {
	case "member_expression":
		return validator.inspectMemberAuthority(node)
	case "call_expression":
		return validator.inspectCallAuthority(node)
	case "identifier":
		return validator.inspectIdentifierAuthority(node)
	}
	return nil
}

func (validator *directCodingBrowserAcceptanceQueryValidator) inspectMemberAuthority(
	member *treesitter.Node,
) error {
	property := member.ChildByFieldName("property")
	object := member.ChildByFieldName("object")
	if property == nil || object == nil {
		return nil
	}
	methodName := validator.text(property)
	objectName := ""
	if object.Kind() == "identifier" {
		objectName = validator.text(object)
	}
	if objectName == "screen" {
		return validator.validateScreenMember(member, methodName)
	}
	if directCodingBrowserKnownScreenQueryMethod(methodName) {
		return fmt.Errorf("browser acceptance query %s must be a direct screen query", methodName)
	}
	if objectName == "fireEvent" {
		return validator.validateFireEventMember(member, methodName)
	}
	return nil
}

func (validator *directCodingBrowserAcceptanceQueryValidator) inspectCallAuthority(
	call *treesitter.Node,
) error {
	callee := call.ChildByFieldName("function")
	if callee == nil {
		return nil
	}
	if callee.Kind() == "identifier" {
		name := validator.text(callee)
		switch name {
		case "expect":
			return validator.validateExpectCall(call)
		case "waitFor":
			return validator.validateWaitForCall(call)
		case "screen", "fireEvent":
			return fmt.Errorf("browser acceptance authority %s cannot be called directly", name)
		default:
			if directCodingBrowserKnownScreenQueryMethod(name) {
				return fmt.Errorf("browser acceptance query %s must be called directly on screen", name)
			}
		}
	}
	if callee.Kind() == "member_expression" {
		object := callee.ChildByFieldName("object")
		if object != nil && object.Kind() == "identifier" && validator.text(object) == "fireEvent" {
			return validator.validateFireEventCall(call, validator.text(callee.ChildByFieldName("property")))
		}
	}
	return nil
}

func (validator *directCodingBrowserAcceptanceQueryValidator) inspectIdentifierAuthority(
	node *treesitter.Node,
) error {
	name := validator.text(node)
	switch name {
	case "screen", "fireEvent":
		parent := node.Parent()
		if parent == nil || parent.Kind() != "member_expression" ||
			!directCodingBrowserSameNode(parent.ChildByFieldName("object"), node) {
			return fmt.Errorf("browser acceptance %s authority requires a direct member query", name)
		}
	case "expect", "waitFor":
		parent := node.Parent()
		if parent == nil || parent.Kind() != "call_expression" ||
			!directCodingBrowserSameNode(parent.ChildByFieldName("function"), node) {
			return fmt.Errorf("browser acceptance %s authority requires a direct call", name)
		}
	}
	return nil
}

func (validator *directCodingBrowserAcceptanceQueryValidator) exactString(
	node *treesitter.Node,
) (string, error) {
	if node == nil || node.Kind() != "string" {
		return "", fmt.Errorf("not an exact string literal")
	}
	raw := validator.text(node)
	if len(raw) < 2 || (raw[0] != '\'' && raw[0] != '"') || raw[len(raw)-1] != raw[0] {
		return "", fmt.Errorf("not an exactly quoted string literal")
	}
	value, err := decodeDirectCodingBrowserJavaScriptString(raw)
	if err != nil {
		return "", err
	}
	if !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("string literal does not decode to canonical display text")
	}
	return value, nil
}

func decodeDirectCodingBrowserJavaScriptString(raw string) (string, error) {
	delimiter := raw[0]
	var quoted strings.Builder
	quoted.Grow(len(raw) + 2)
	quoted.WriteByte('"')
	for index := 1; index < len(raw)-1; index++ {
		character := raw[index]
		if character == '\r' || character == '\n' {
			return "", fmt.Errorf("string literal contains an unescaped line break")
		}
		if character == '\\' {
			if index+1 >= len(raw)-1 {
				return "", fmt.Errorf("string literal has an incomplete escape")
			}
			next := raw[index+1]
			index++
			switch {
			case next == delimiter || next == '/':
				if next == '"' {
					quoted.WriteString(`\"`)
				} else {
					quoted.WriteByte(next)
				}
			case next == '\'':
				quoted.WriteByte(next)
			case next == '"':
				quoted.WriteString(`\"`)
			default:
				quoted.WriteByte('\\')
				quoted.WriteByte(next)
			}
			continue
		}
		if character == '"' {
			quoted.WriteString(`\"`)
		} else {
			quoted.WriteByte(character)
		}
	}
	quoted.WriteByte('"')
	value, err := strconv.Unquote(quoted.String())
	if err != nil {
		return "", fmt.Errorf("string literal uses an unsupported escape: %w", err)
	}
	return value, nil
}

func (validator *directCodingBrowserAcceptanceQueryValidator) text(node *treesitter.Node) string {
	if node == nil {
		return ""
	}
	return string(validator.source[node.StartByte():node.EndByte()])
}

func directCodingBrowserSameNode(first *treesitter.Node, second *treesitter.Node) bool {
	return first != nil && second != nil && first.Id() == second.Id()
}
