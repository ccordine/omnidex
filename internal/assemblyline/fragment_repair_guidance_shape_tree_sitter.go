package assemblyline

import (
	"fmt"
	"strings"
	"unsafe"

	treesitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

func fragmentRepairGuidanceContainsBoundedDeclaration(
	language boundedSourceLanguage,
	instruction string,
) (bool, error) {
	return fragmentRepairGuidanceTreeContainsDeclaration(
		language.display, language.fragmentLanguage, language.declarationKinds, instruction,
	)
}

func fragmentRepairGuidanceContainsTypeScriptDeclaration(
	instruction string,
) (bool, error) {
	kinds := sourceNodeKinds("function_declaration")
	for _, grammar := range []struct {
		label   string
		pointer func() unsafe.Pointer
	}{
		{label: "TypeScript", pointer: typescript.LanguageTypescript},
		{label: "TSX", pointer: typescript.LanguageTSX},
	} {
		contains, err := fragmentRepairGuidanceTreeContainsDeclaration(
			grammar.label, grammar.pointer, kinds, instruction,
		)
		if err != nil || contains {
			return contains, err
		}
	}
	return false, nil
}

// Tree-sitter retains valid subtrees beneath an error-recovering prose root.
// A supported declaration node is source evidence whenever its complete
// subtree and body are parser-valid. Quoting cannot turn a complete source
// declaration into instruction-only authority.
func fragmentRepairGuidanceTreeContainsDeclaration(
	label string,
	languagePointer func() unsafe.Pointer,
	declarationKinds map[string]struct{},
	instruction string,
) (bool, error) {
	parser := treesitter.NewParser()
	if err := parser.SetLanguage(treesitter.NewLanguage(languagePointer())); err != nil {
		parser.Close()
		return false, fmt.Errorf("configure %s repair-guidance parser: %w", label, err)
	}
	defer parser.Close()
	tree := parser.Parse([]byte(instruction), nil)
	if tree == nil {
		return false, fmt.Errorf("%s repair-guidance parser returned no syntax tree", label)
	}
	defer tree.Close()
	return fragmentRepairGuidanceNodeContainsDeclaration(
		tree.RootNode(), declarationKinds,
	), nil
}

func fragmentRepairGuidanceNodeContainsDeclaration(
	node *treesitter.Node,
	declarationKinds map[string]struct{},
) bool {
	if node == nil {
		return false
	}
	if _, supported := declarationKinds[node.Kind()]; supported && !node.HasError() {
		body := node.ChildByFieldName("body")
		if body != nil && !body.IsMissing() && !body.HasError() {
			return true
		}
	}
	for index := uint(0); index < node.ChildCount(); index++ {
		if fragmentRepairGuidanceNodeContainsDeclaration(
			node.Child(index), declarationKinds,
		) {
			return true
		}
	}
	return false
}

type fragmentRepairGuidanceBodyGrammar struct {
	label     string
	pointer   func() unsafe.Pointer
	prefix    string
	suffix    string
	topKind   string
	ownerKind string
	ownerName string
	bodyKind  string
}

func fragmentRepairGuidanceIsWholeTypeScriptSourceBody(
	source string,
) (bool, error) {
	for _, grammar := range []fragmentRepairGuidanceBodyGrammar{
		fragmentRepairGuidanceTypeScriptBodyGrammar(
			"TypeScript", typescript.LanguageTypescript,
		),
		fragmentRepairGuidanceTypeScriptBodyGrammar("TSX", typescript.LanguageTSX),
	} {
		contains, err := fragmentRepairGuidanceIsWholeTreeSourceBody(grammar, source)
		if err != nil || contains {
			return contains, err
		}
	}
	return false, nil
}

func fragmentRepairGuidanceTypeScriptBodyGrammar(
	label string,
	pointer func() unsafe.Pointer,
) fragmentRepairGuidanceBodyGrammar {
	return fragmentRepairGuidanceBodyGrammar{
		label: label, pointer: pointer,
		prefix: "function omnidexRepairInstruction(){\n", suffix: "\n}",
		topKind: "function_declaration", ownerKind: "function_declaration",
		ownerName: "omnidexRepairInstruction", bodyKind: "statement_block",
	}
}

func fragmentRepairGuidanceIsWholeBoundedSourceBody(
	language boundedSourceLanguage,
	source string,
) (bool, error) {
	var grammar fragmentRepairGuidanceBodyGrammar
	switch language.id {
	case "javascript":
		grammar = fragmentRepairGuidanceBodyGrammar{
			label: language.display, pointer: language.fragmentLanguage,
			prefix: "function omnidexRepairInstruction(){\n", suffix: "\n}",
			topKind: "function_declaration", ownerKind: "function_declaration",
			ownerName: "omnidexRepairInstruction", bodyKind: "statement_block",
		}
	case "java":
		grammar = fragmentRepairGuidanceBodyGrammar{
			label: language.display, pointer: language.fragmentLanguage,
			prefix: "class OmnidexRepairInstruction { void repair(){\n", suffix: "\n} }",
			topKind: "class_declaration", ownerKind: "method_declaration",
			ownerName: "repair", bodyKind: "block",
		}
	case "rust":
		grammar = fragmentRepairGuidanceBodyGrammar{
			label: language.display, pointer: language.fragmentLanguage,
			prefix: "fn omnidex_repair_instruction(){\n", suffix: "\n}",
			topKind: "function_item", ownerKind: "function_item",
			ownerName: "omnidex_repair_instruction", bodyKind: "block",
		}
	case "php":
		grammar = fragmentRepairGuidanceBodyGrammar{
			label: language.display, pointer: language.fragmentLanguage,
			prefix: "function omnidexRepairInstruction(){\n", suffix: "\n}",
			topKind: "function_definition", ownerKind: "function_definition",
			ownerName: "omnidexRepairInstruction", bodyKind: "compound_statement",
		}
	default:
		return false, fmt.Errorf(
			"%s has no repair-guidance body grammar", language.display,
		)
	}
	return fragmentRepairGuidanceIsWholeTreeSourceBody(grammar, source)
}

func fragmentRepairGuidanceIsWholeTreeSourceBody(
	grammar fragmentRepairGuidanceBodyGrammar,
	source string,
) (bool, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return false, nil
	}
	wrapped := grammar.prefix + source + grammar.suffix
	parser := treesitter.NewParser()
	if err := parser.SetLanguage(treesitter.NewLanguage(grammar.pointer())); err != nil {
		parser.Close()
		return false, fmt.Errorf(
			"configure %s repair-guidance body parser: %w", grammar.label, err,
		)
	}
	defer parser.Close()
	tree := parser.Parse([]byte(wrapped), nil)
	if tree == nil {
		return false, fmt.Errorf(
			"%s repair-guidance body parser returned no syntax tree", grammar.label,
		)
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() || root.NamedChildCount() != 1 {
		return false, nil
	}
	top := root.NamedChild(0)
	if top == nil || top.Kind() != grammar.topKind ||
		int(top.StartByte()) != 0 || int(top.EndByte()) != len(wrapped) {
		return false, nil
	}
	owner, count := fragmentRepairGuidanceNamedOwner(
		top, grammar.ownerKind, grammar.ownerName, []byte(wrapped),
	)
	if count != 1 || owner == nil {
		return false, nil
	}
	body := owner.ChildByFieldName("body")
	if body == nil || body.IsMissing() || body.HasError() || body.Kind() != grammar.bodyKind ||
		int(body.StartByte()) != len(grammar.prefix)-2 ||
		int(body.EndByte()) != len(grammar.prefix)+len(source)+2 {
		return false, nil
	}
	for index := uint(0); index < body.NamedChildCount(); index++ {
		child := body.NamedChild(index)
		if child != nil && child.Kind() != "comment" {
			return true, nil
		}
	}
	return false, nil
}

func fragmentRepairGuidanceNamedOwner(
	node *treesitter.Node,
	kind string,
	name string,
	source []byte,
) (*treesitter.Node, int) {
	if node == nil {
		return nil, 0
	}
	var found *treesitter.Node
	count := 0
	if node.Kind() == kind {
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil && nameNode.Utf8Text(source) == name {
			found, count = node, 1
		}
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		candidate, nested := fragmentRepairGuidanceNamedOwner(
			node.NamedChild(index), kind, name, source,
		)
		if nested > 0 {
			found = candidate
			count += nested
		}
	}
	return found, count
}
