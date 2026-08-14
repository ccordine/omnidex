package assemblyline

import (
	"errors"
	"fmt"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

type TypeScriptFunctionContract struct {
	Signature string
	TSX       bool
	Policy    TypeScriptFunctionPolicy
}

type TypeScriptFragment struct {
	Name   string
	API    string
	Source string
}

type TypeScriptSyntaxFailure struct {
	Kind   string
	Line   int
	Column int
}

func (failure TypeScriptSyntaxFailure) Error() string {
	return fmt.Sprintf("%s at line %d column %d", failure.Kind, failure.Line, failure.Column)
}

func TypeScriptSyntaxFailureFromError(err error) (TypeScriptSyntaxFailure, bool) {
	var failure TypeScriptSyntaxFailure
	if !errors.As(err, &failure) {
		return TypeScriptSyntaxFailure{}, false
	}
	return failure, true
}

func ParseTypeScriptFunction(contract TypeScriptFunctionContract, raw string) (TypeScriptFragment, error) {
	var zero TypeScriptFragment
	signature := strings.TrimSpace(contract.Signature)
	if signature == "" || strings.ContainsAny(signature, "\r\n") {
		return zero, fmt.Errorf("TypeScript function contract requires one single-line signature")
	}
	content := strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if content == "" {
		return zero, fmt.Errorf("TypeScript fragment is empty")
	}
	if err := validateTypeScriptFunctionPolicy(contract.Policy); err != nil {
		return zero, fmt.Errorf("TypeScript function policy: %w", err)
	}
	actual, closeActual, err := parseSingleTypeScriptFunction(content, contract.TSX, true, contract.Policy)
	if err != nil {
		return zero, err
	}
	defer closeActual()
	expectedSource := signature + " {}"
	expected, closeExpected, err := parseSingleTypeScriptFunction(expectedSource, contract.TSX, false, TypeScriptFunctionPolicy{})
	if err != nil {
		return zero, fmt.Errorf("invalid code-owned TypeScript signature: %w", err)
	}
	defer closeExpected()
	if actual.shape != expected.shape {
		return zero, fmt.Errorf("TypeScript fragment declaration does not match required signature %q", signature)
	}
	return TypeScriptFragment{
		Name: actual.name, API: signature, Source: content + "\n",
	}, nil
}

type parsedTypeScriptFunction struct {
	name  string
	shape string
}

func parseSingleTypeScriptFunction(
	source string,
	tsx bool,
	requireExecutableBodies bool,
	policy TypeScriptFunctionPolicy,
) (parsedTypeScriptFunction, func(), error) {
	parser := treesitter.NewParser()
	languagePointer := typescript.LanguageTypescript()
	if tsx {
		languagePointer = typescript.LanguageTSX()
	}
	if err := parser.SetLanguage(treesitter.NewLanguage(languagePointer)); err != nil {
		parser.Close()
		return parsedTypeScriptFunction{}, func() {}, fmt.Errorf("configure TypeScript parser: %w", err)
	}
	tree := parser.Parse([]byte(source), nil)
	closeAll := func() {
		if tree != nil {
			tree.Close()
		}
		parser.Close()
	}
	if tree == nil {
		closeAll()
		return parsedTypeScriptFunction{}, func() {}, fmt.Errorf("TypeScript parser returned no syntax tree")
	}
	root := tree.RootNode()
	if root.HasError() {
		detail := firstTypeScriptSyntaxFailure(root)
		closeAll()
		return parsedTypeScriptFunction{}, func() {}, fmt.Errorf("TypeScript syntax rejected: %w", detail)
	}
	if root.NamedChildCount() != 1 {
		closeAll()
		return parsedTypeScriptFunction{}, func() {}, fmt.Errorf("TypeScript fragment must contain exactly one declaration")
	}
	declaration := root.NamedChild(0)
	if declaration == nil || declaration.Kind() != "function_declaration" {
		kind := "missing"
		if declaration != nil {
			kind = declaration.Kind()
		}
		closeAll()
		return parsedTypeScriptFunction{}, func() {}, fmt.Errorf("TypeScript fragment must be one raw function declaration, received %s", kind)
	}
	name := declaration.ChildByFieldName("name")
	body := declaration.ChildByFieldName("body")
	if name == nil || body == nil {
		closeAll()
		return parsedTypeScriptFunction{}, func() {}, fmt.Errorf("TypeScript function requires a name and body")
	}
	if requireExecutableBodies {
		if err := validateTypeScriptGeneratedNode(declaration); err != nil {
			closeAll()
			return parsedTypeScriptFunction{}, func() {}, err
		}
		if err := validateTypeScriptFunctionPolicyNode(declaration, []byte(source), policy); err != nil {
			closeAll()
			return parsedTypeScriptFunction{}, func() {}, err
		}
	}
	shape := canonicalTypeScriptNode(declaration, body.Id(), []byte(source))
	return parsedTypeScriptFunction{name: name.Utf8Text([]byte(source)), shape: shape}, closeAll, nil
}

func validateTypeScriptGeneratedNode(node *treesitter.Node) error {
	if node == nil {
		return nil
	}
	if node.Kind() == "comment" {
		return newTypeScriptFragmentViolation(
			TypeScriptViolationComment,
			"TypeScript fragment comments are forbidden; return executable code only",
			"Delete every comment node from the current declaration. Replace a comment that stands in for behavior with executable code, or remove it if no behavior is required. Change nothing unrelated.",
		)
	}
	if node.Kind() == "statement_block" && !hasExecutableTypeScriptChild(node) {
		position := node.StartPosition()
		return newTypeScriptFragmentViolation(
			TypeScriptViolationEmptyBody,
			fmt.Sprintf(
				"TypeScript fragment contains an empty executable body at line %d column %d",
				position.Row+1, position.Column+1,
			),
			"Implement every empty function or callback body with executable code required by the current declaration, or remove the unused empty function or callback. Change nothing unrelated.",
		)
	}
	for index := uint(0); index < node.ChildCount(); index++ {
		if err := validateTypeScriptGeneratedNode(node.Child(index)); err != nil {
			return err
		}
	}
	return nil
}

func hasExecutableTypeScriptChild(node *treesitter.Node) bool {
	for index := uint(0); index < node.NamedChildCount(); index++ {
		child := node.NamedChild(index)
		if child != nil && child.Kind() != "comment" {
			return true
		}
	}
	return false
}

func canonicalTypeScriptNode(node *treesitter.Node, skippedID uintptr, source []byte) string {
	if node == nil || node.Id() == skippedID {
		return ""
	}
	var output strings.Builder
	output.WriteByte('(')
	output.WriteString(node.Kind())
	if node.ChildCount() == 0 {
		output.WriteByte(':')
		output.WriteString(node.Utf8Text(source))
	} else {
		for index := uint(0); index < node.ChildCount(); index++ {
			output.WriteString(canonicalTypeScriptNode(node.Child(index), skippedID, source))
		}
	}
	output.WriteByte(')')
	return output.String()
}

func firstTypeScriptSyntaxFailure(root *treesitter.Node) TypeScriptSyntaxFailure {
	if root == nil {
		return TypeScriptSyntaxFailure{Kind: "unknown parser failure", Line: 1, Column: 1}
	}
	if root.IsError() || root.IsMissing() {
		position := root.StartPosition()
		return TypeScriptSyntaxFailure{Kind: root.Kind(), Line: int(position.Row) + 1, Column: int(position.Column) + 1}
	}
	for index := uint(0); index < root.ChildCount(); index++ {
		child := root.Child(index)
		if child != nil && child.HasError() {
			return firstTypeScriptSyntaxFailure(child)
		}
	}
	position := root.StartPosition()
	return TypeScriptSyntaxFailure{Kind: "invalid syntax", Line: int(position.Row) + 1, Column: int(position.Column) + 1}
}

func ValidateTypeScriptSource(source string, tsx bool) error {
	parser := treesitter.NewParser()
	languagePointer := typescript.LanguageTypescript()
	if tsx {
		languagePointer = typescript.LanguageTSX()
	}
	if err := parser.SetLanguage(treesitter.NewLanguage(languagePointer)); err != nil {
		parser.Close()
		return fmt.Errorf("configure TypeScript parser: %w", err)
	}
	defer parser.Close()
	tree := parser.Parse([]byte(source), nil)
	if tree == nil {
		return fmt.Errorf("TypeScript parser returned no syntax tree")
	}
	defer tree.Close()
	if root := tree.RootNode(); root.HasError() {
		return fmt.Errorf("TypeScript syntax rejected: %w", firstTypeScriptSyntaxFailure(root))
	}
	return nil
}
