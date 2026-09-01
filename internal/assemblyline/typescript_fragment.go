package assemblyline

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/sourcebodyresponse"
	treesitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

type TypeScriptFunctionContract struct {
	Signature string
	TSX       bool
	Policy    SourceFunctionPolicy
}

type TypeScriptFragment struct {
	Name   string
	API    string
	Source string
}

type TypeScriptSyntaxFailure struct {
	Kind      string
	Line      int
	Column    int
	StartByte int
	EndByte   int
}

func (failure TypeScriptSyntaxFailure) Error() string {
	return fmt.Sprintf("%s at line %d column %d", failure.Kind, failure.Line, failure.Column)
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
	if err := validateSourceFunctionPolicy(contract.Policy); err != nil {
		return zero, fmt.Errorf("TypeScript function policy: %w", err)
	}
	actual, closeActual, err := parseSingleTypeScriptFunction(content, contract.TSX, true, contract.Policy)
	if err != nil {
		return zero, err
	}
	defer closeActual()
	expectedSource := signature + " {}"
	expected, closeExpected, err := parseSingleTypeScriptFunction(expectedSource, contract.TSX, false, SourceFunctionPolicy{})
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

// ParseTypeScriptFunctionBody applies an ordinary source-body response inside
// the exact declaration owned by code, then delegates every syntax, signature,
// policy, and scope-sensitive check to the established parser boundary.
func ParseTypeScriptFunctionBody(
	contract TypeScriptFunctionContract,
	rawBody string,
) (TypeScriptFragment, error) {
	body, err := ExtractTypeScriptFunctionBodyResponse(contract, rawBody)
	if err != nil {
		return TypeScriptFragment{}, fmt.Errorf("TypeScript source body: %w", err)
	}
	declaration, err := ComposeSourceDeclaration(contract.Signature, body)
	if err != nil {
		return TypeScriptFragment{}, fmt.Errorf("TypeScript source body: %w", err)
	}
	_, closeExpected, err := parseSingleTypeScriptFunction(
		strings.TrimSpace(contract.Signature)+" {}",
		contract.TSX,
		false,
		SourceFunctionPolicy{},
	)
	if err != nil {
		return TypeScriptFragment{}, fmt.Errorf("invalid code-owned TypeScript signature: %w", err)
	}
	closeExpected()
	fragment, err := ParseTypeScriptFunction(contract, declaration)
	if err == nil {
		return fragment, nil
	}
	prefix := strings.TrimSpace(contract.Signature) + " {\n"
	if declaration != prefix+body+"\n}" {
		return TypeScriptFragment{}, fmt.Errorf(
			"TypeScript validator could not prove the code-owned declaration projection: %w",
			err,
		)
	}
	bodyStart, bodyEnd := len(prefix), len(prefix)+len(body)
	startByte, endByte := 0, 0
	question := ""
	identifierFailure := false
	var syntaxFailure TypeScriptSyntaxFailure
	if errors.As(err, &syntaxFailure) {
		startByte, endByte = syntaxFailure.StartByte, syntaxFailure.EndByte
		question = "What should replace this syntactically invalid span?"
	} else {
		var violation *TypeScriptFragmentViolation
		if !errors.As(err, &violation) ||
			violation.Code != TypeScriptViolationForbiddenIdentifier {
			return TypeScriptFragment{}, err
		}
		startByte, endByte = violation.StartByte, violation.EndByte
		question = "Which available value has the meaning required at this unavailable reference?"
		identifierFailure = true
	}
	if startByte < bodyStart || endByte > bodyEnd || startByte >= endByte {
		return TypeScriptFragment{}, err
	}
	var defect *SourceBodyDefect
	var defectErr error
	startByte -= bodyStart
	endByte -= bodyStart
	if startByte == 0 && endByte == len(body) {
		return TypeScriptFragment{}, err
	}
	if identifierFailure {
		defect, defectErr = NewSourceBodyIdentifierDefect(
			body,
			startByte,
			endByte,
			question,
			err,
			nil,
		)
	} else {
		defect, defectErr = NewSourceBodyDefect(
			body,
			startByte,
			endByte,
			question,
			err,
		)
	}
	if defectErr != nil {
		return TypeScriptFragment{}, fmt.Errorf(
			"map exact TypeScript validation node to implementation body: %w",
			defectErr,
		)
	}
	return TypeScriptFragment{}, defect
}

// ExtractTypeScriptFunctionBodyResponse treats a complete declaration as
// ordinary redundant source. Code extracts only its body and validates that
// body under the authoritative contract signature. When declaration-shaped
// source is present, code accepts only one direct block-bodied callable across
// every fenced region; volunteered names, types, and parameters are irrelevant.
// A single fenced body is tolerated only when it is already parseable under
// that signature, preventing prose from becoming an inferred correction span.
func ExtractTypeScriptFunctionBodyResponse(
	contract TypeScriptFunctionContract,
	raw string,
) (string, error) {
	candidates, err := sourcebodyresponse.ExtractCandidates(raw, MaxPortableRawCandidateBytes)
	if err != nil {
		return "", err
	}
	declarationBodies := make([]string, 0, len(candidates))
	requiresCallableExtraction := len(candidates) > 1
	for _, candidate := range candidates {
		bodies, structured, extractErr := extractTypeScriptDeclarationBodies(
			candidate.Source,
			contract.TSX,
		)
		if extractErr != nil {
			return "", extractErr
		}
		requiresCallableExtraction = requiresCallableExtraction || structured
		declarationBodies = append(declarationBodies, bodies...)
	}
	if requiresCallableExtraction {
		if len(declarationBodies) != 1 {
			return "", fmt.Errorf(
				"fenced TypeScript response contains %d direct block-bodied callable candidates; exactly one is required for deterministic extraction",
				len(declarationBodies),
			)
		}
		return NormalizeSourceBodyResponse(declarationBodies[0])
	}
	candidate := candidates[0]
	if candidate.Fenced {
		assembled, err := ComposeSourceDeclaration(contract.Signature, candidate.Source)
		if err != nil {
			return "", err
		}
		_, closeBody, parseErr := parseSingleTypeScriptFunction(
			assembled, contract.TSX, false, SourceFunctionPolicy{},
		)
		if parseErr != nil {
			return "", fmt.Errorf(
				"fenced TypeScript response contains neither one declaration nor one parseable implementation body: %w",
				parseErr,
			)
		}
		closeBody()
	}
	return NormalizeSourceBodyResponse(candidate.Source)
}

func extractTypeScriptDeclarationBodies(source string, tsx bool) ([]string, bool, error) {
	parser := treesitter.NewParser()
	languagePointer := typescript.LanguageTypescript()
	if tsx {
		languagePointer = typescript.LanguageTSX()
	}
	if err := parser.SetLanguage(treesitter.NewLanguage(languagePointer)); err != nil {
		parser.Close()
		return nil, false, fmt.Errorf("configure TypeScript extraction parser: %w", err)
	}
	tree := parser.Parse([]byte(source), nil)
	if tree == nil {
		parser.Close()
		return nil, false, fmt.Errorf("TypeScript extraction parser returned no syntax tree")
	}
	defer tree.Close()
	defer parser.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() {
		return nil, false, nil
	}
	bodies := make([]string, 0, 1)
	moduleMarker := false
	directExecution := false
	for index := uint(0); index < root.NamedChildCount(); index++ {
		top := root.NamedChild(index)
		if top == nil {
			continue
		}
		if top.Kind() == "import_statement" || top.Kind() == "export_statement" {
			moduleMarker = true
		}
		nodes := directTypeScriptCallableDeclarations(top)
		switch top.Kind() {
		case "import_statement", "export_statement", "function_declaration",
			"type_alias_declaration", "interface_declaration", "enum_declaration":
		case "lexical_declaration", "variable_declaration":
			if len(nodes) == 0 || len(nodes) != directTypeScriptVariableDeclaratorCount(top) {
				directExecution = true
			}
		default:
			directExecution = true
		}
		for _, declaration := range nodes {
			body := declaration.ChildByFieldName("body")
			if declaration.Kind() == "variable_declarator" {
				value := declaration.ChildByFieldName("value")
				if value == nil || (value.Kind() != "arrow_function" && value.Kind() != "function_expression") {
					continue
				}
				body = value.ChildByFieldName("body")
			}
			if body == nil || body.Kind() != "statement_block" {
				continue
			}
			start, end := int(body.StartByte()), int(body.EndByte())
			if start < 0 || end <= start+1 || end > len(source) || source[start] != '{' || source[end-1] != '}' {
				return nil, false, fmt.Errorf("TypeScript declaration body range is invalid")
			}
			bodies = append(bodies, source[start+1:end-1])
		}
	}
	structured := moduleMarker || len(bodies) > 0 && !directExecution
	if !structured {
		return nil, false, nil
	}
	return bodies, true, nil
}

func directTypeScriptVariableDeclaratorCount(declaration *treesitter.Node) int {
	if declaration == nil || (declaration.Kind() != "lexical_declaration" &&
		declaration.Kind() != "variable_declaration") {
		return 0
	}
	count := 0
	for index := uint(0); index < declaration.NamedChildCount(); index++ {
		child := declaration.NamedChild(index)
		if child != nil && child.Kind() == "variable_declarator" {
			count++
		}
	}
	return count
}

func directTypeScriptCallableDeclarations(top *treesitter.Node) []*treesitter.Node {
	if top == nil {
		return nil
	}
	switch top.Kind() {
	case "function_declaration":
		return []*treesitter.Node{top}
	case "lexical_declaration", "variable_declaration":
		declarations := make([]*treesitter.Node, 0, top.NamedChildCount())
		for index := uint(0); index < top.NamedChildCount(); index++ {
			child := top.NamedChild(index)
			if child != nil && child.Kind() == "variable_declarator" {
				value := child.ChildByFieldName("value")
				if value != nil && (value.Kind() == "arrow_function" || value.Kind() == "function_expression") {
					declarations = append(declarations, child)
				}
			}
		}
		return declarations
	case "export_statement":
		declarations := make([]*treesitter.Node, 0, top.NamedChildCount())
		for index := uint(0); index < top.NamedChildCount(); index++ {
			declarations = append(declarations, directTypeScriptCallableDeclarations(top.NamedChild(index))...)
		}
		return declarations
	default:
		return nil
	}
}

type parsedTypeScriptFunction struct {
	name  string
	shape string
}

func parseSingleTypeScriptFunction(
	source string,
	tsx bool,
	requireExecutableBodies bool,
	policy SourceFunctionPolicy,
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
		detail, located := firstTypeScriptSyntaxFailure(root)
		closeAll()
		if !located {
			return parsedTypeScriptFunction{}, func() {}, fmt.Errorf(
				"TypeScript syntax rejected without one exact non-empty parser-error leaf",
			)
		}
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
	if int(declaration.StartByte()) != 0 || int(declaration.EndByte()) != len(source) {
		closeAll()
		return parsedTypeScriptFunction{}, func() {}, fmt.Errorf(
			"TypeScript fragment must contain only one exact raw function declaration",
		)
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
	if node.Kind() == "statement_block" && !hasExecutableTypeScriptChild(node) {
		position := node.StartPosition()
		return newTypeScriptFragmentViolation(
			TypeScriptViolationEmptyBody,
			fmt.Sprintf(
				"TypeScript fragment contains an empty executable body at line %d column %d",
				position.Row+1, position.Column+1,
			),
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

func firstTypeScriptSyntaxFailure(root *treesitter.Node) (TypeScriptSyntaxFailure, bool) {
	leaf := smallestNonemptyParserErrorLeaf(root)
	if leaf == nil {
		return TypeScriptSyntaxFailure{}, false
	}
	position := leaf.StartPosition()
	return TypeScriptSyntaxFailure{
		Kind: leaf.Kind(), Line: int(position.Row) + 1, Column: int(position.Column) + 1,
		StartByte: int(leaf.StartByte()), EndByte: int(leaf.EndByte()),
	}, true
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
		failure, located := firstTypeScriptSyntaxFailure(root)
		if !located {
			return fmt.Errorf(
				"TypeScript syntax rejected without one exact non-empty parser-error leaf",
			)
		}
		return fmt.Errorf("TypeScript syntax rejected: %w", failure)
	}
	return nil
}
