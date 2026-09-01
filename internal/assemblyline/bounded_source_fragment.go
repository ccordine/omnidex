package assemblyline

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
	"unsafe"

	"github.com/gryph/omnidex/internal/sourcebodyresponse"
	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func validateBoundedSourceFragment(
	language boundedSourceLanguage,
	signature string,
	responseBody string,
) (string, error) {
	signature = strings.TrimSpace(signature)
	if signature == "" || strings.ContainsAny(signature, "\x00\r\n") ||
		!utf8.ValidString(signature) || len(signature) > 1024 {
		return "", fmt.Errorf("%s fragment signature must be one trimmed line", language.display)
	}
	normalizedBody, err := extractBoundedSourceBodyResponse(
		language, signature, responseBody,
	)
	if err != nil {
		return "", fmt.Errorf("%s source body: %w", language.display, err)
	}
	declaration, err := ComposeSourceDeclaration(signature, normalizedBody)
	if err != nil {
		return "", fmt.Errorf("%s source body: %w", language.display, err)
	}
	if _, err := boundedSourceDeclarationShape(language, signature+" {}"); err != nil {
		return "", fmt.Errorf("invalid code-owned %s signature: %w", language.display, err)
	}
	validated, err := validateBoundedSourceDeclaration(language, signature, declaration)
	if err == nil {
		return validated, nil
	}
	var syntaxFailure *boundedSourceSyntaxFailure
	if !errors.As(err, &syntaxFailure) {
		return "", err
	}
	prefix := signature + " {\n"
	bodyStart := len(prefix)
	bodyEnd := bodyStart + len(normalizedBody)
	if declaration != prefix+normalizedBody+"\n}" ||
		syntaxFailure.startByte < bodyStart || syntaxFailure.endByte > bodyEnd ||
		syntaxFailure.startByte >= syntaxFailure.endByte {
		return "", err
	}
	startByte := syntaxFailure.startByte - bodyStart
	endByte := syntaxFailure.endByte - bodyStart
	if startByte == 0 && endByte == len(normalizedBody) {
		return "", err
	}
	defect, defectErr := NewSourceBodyDefect(
		normalizedBody,
		startByte,
		endByte,
		"What should replace this syntactically invalid span?",
		err,
	)
	if defectErr != nil {
		return "", fmt.Errorf("map exact %s syntax node to implementation body: %w", language.display, defectErr)
	}
	return "", defect
}

func extractBoundedSourceBodyResponse(
	language boundedSourceLanguage,
	signature string,
	raw string,
) (string, error) {
	candidate, err := sourcebodyresponse.ExtractCandidate(raw, MaxPortableRawCandidateBytes)
	if err != nil {
		return "", err
	}
	body, declaration, err := boundedSourceDeclarationBody(language, candidate.Source)
	if err != nil {
		return "", err
	}
	if declaration {
		return NormalizeSourceBodyResponse(body)
	}
	if candidate.Fenced {
		assembled, err := ComposeSourceDeclaration(signature, candidate.Source)
		if err != nil {
			return "", err
		}
		if _, err := boundedSourceDeclarationShape(language, assembled); err != nil {
			return "", fmt.Errorf(
				"fenced %s response contains neither one declaration nor one parseable implementation body: %w",
				language.display, err,
			)
		}
	}
	return NormalizeSourceBodyResponse(candidate.Source)
}

func boundedSourceDeclarationBody(
	language boundedSourceLanguage,
	source string,
) (string, bool, error) {
	parser := treesitter.NewParser()
	if err := parser.SetLanguage(treesitter.NewLanguage(language.fragmentLanguage())); err != nil {
		parser.Close()
		return "", false, fmt.Errorf("configure %s extraction parser: %w", language.display, err)
	}
	tree := parser.Parse([]byte(source), nil)
	if tree == nil {
		parser.Close()
		return "", false, fmt.Errorf("%s extraction parser returned no syntax tree", language.display)
	}
	defer tree.Close()
	defer parser.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() || root.NamedChildCount() != 1 {
		return "", false, nil
	}
	top := root.NamedChild(0)
	if top == nil || int(top.StartByte()) != 0 || int(top.EndByte()) != len(source) {
		return "", false, nil
	}
	declaration := top
	if language.allowCodeOwnedExport && top.Kind() == "export_statement" {
		declaration = nil
		for index := uint(0); index < top.NamedChildCount(); index++ {
			child := top.NamedChild(index)
			if child == nil {
				continue
			}
			if _, allowed := language.declarationKinds[child.Kind()]; allowed {
				if declaration != nil {
					return "", false, nil
				}
				declaration = child
			}
		}
	}
	if declaration == nil {
		return "", false, nil
	}
	if _, allowed := language.declarationKinds[declaration.Kind()]; !allowed {
		return "", false, nil
	}
	body := declaration.ChildByFieldName("body")
	if body == nil {
		return "", false, nil
	}
	start, end := int(body.StartByte()), int(body.EndByte())
	if start < 0 || end <= start+1 || end > len(source) || source[start] != '{' || source[end-1] != '}' {
		return "", false, fmt.Errorf("%s declaration body range is invalid", language.display)
	}
	return source[start+1 : end-1], true, nil
}

func validateBoundedSourceDeclaration(
	language boundedSourceLanguage,
	signature string,
	declaration string,
) (string, error) {
	content, actual, err := projectBoundedSourceDeclaration(language, declaration)
	if err != nil {
		return "", err
	}
	expected, err := boundedSourceDeclarationShape(language, signature+" {}")
	if err != nil {
		return "", fmt.Errorf("invalid code-owned %s signature: %w", language.display, err)
	}
	if actual != expected {
		return "", fmt.Errorf(
			"%s fragment declaration does not match required signature %q",
			language.display, signature,
		)
	}
	return content, nil
}

func projectBoundedSourceDeclaration(
	language boundedSourceLanguage,
	candidate string,
) (string, string, error) {
	content := candidate
	if content == "" {
		return "", "", fmt.Errorf("%s fragment is empty", language.display)
	}
	if !utf8.ValidString(content) || strings.ContainsRune(content, '\x00') {
		return "", "", fmt.Errorf("%s fragment must be valid UTF-8 without NUL bytes", language.display)
	}
	if len(content) > maxPortableCandidateBytes {
		return "", "", fmt.Errorf(
			"%s fragment exceeds %d bytes", language.display, maxPortableCandidateBytes,
		)
	}
	actual, err := boundedSourceDeclarationShape(language, content)
	if err != nil {
		return "", "", err
	}
	return content, actual, nil
}

func boundedSourceDeclarationShape(
	language boundedSourceLanguage,
	source string,
) (string, error) {
	parser, tree, err := parseBoundedSourceTree(language, language.fragmentLanguage, source)
	if err != nil {
		return "", err
	}
	defer parser.Close()
	defer tree.Close()
	root := tree.RootNode()
	if root.NamedChildCount() != 1 {
		return "", fmt.Errorf(
			"%s fragment must contain exactly one top-level declaration", language.display,
		)
	}
	top := root.NamedChild(0)
	if int(top.StartByte()) != 0 || int(top.EndByte()) != len(source) {
		return "", fmt.Errorf(
			"%s fragment must contain only one exact top-level declaration", language.display,
		)
	}
	declaration, err := boundedSourceDeclarationNode(language, top)
	if err != nil {
		return "", err
	}
	body := declaration.ChildByFieldName("body")
	if body == nil {
		return "", fmt.Errorf("%s %s requires one body", language.display, language.declaration)
	}
	return canonicalBoundedSourceNode(top, body.Id(), []byte(source)), nil
}

func boundedSourceDeclarationNode(
	language boundedSourceLanguage,
	top *treesitter.Node,
) (*treesitter.Node, error) {
	if top == nil {
		return nil, fmt.Errorf("%s fragment has no declaration", language.display)
	}
	if _, allowed := language.declarationKinds[top.Kind()]; allowed {
		return top, nil
	}
	return nil, fmt.Errorf(
		"%s fragment must be one supported raw %s, received %s",
		language.display, language.declaration, top.Kind(),
	)
}

func parseBoundedSourceTree(
	language boundedSourceLanguage,
	languagePointer func() unsafe.Pointer,
	source string,
) (*treesitter.Parser, *treesitter.Tree, error) {
	parser := treesitter.NewParser()
	if err := parser.SetLanguage(treesitter.NewLanguage(languagePointer())); err != nil {
		parser.Close()
		return nil, nil, fmt.Errorf("configure %s parser: %w", language.display, err)
	}
	tree := parser.Parse([]byte(source), nil)
	if tree == nil {
		parser.Close()
		return nil, nil, fmt.Errorf("%s parser returned no syntax tree", language.display)
	}
	if root := tree.RootNode(); root.HasError() {
		detail := firstBoundedSourceSyntaxFailure(root)
		tree.Close()
		parser.Close()
		if detail == nil {
			return nil, nil, fmt.Errorf(
				"%s syntax rejected without one exact non-empty parser-error leaf",
				language.display,
			)
		}
		return nil, nil, fmt.Errorf("%s syntax rejected: %w", language.display, detail)
	}
	return parser, tree, nil
}

type boundedSourceSyntaxFailure struct {
	kind      string
	line      int
	column    int
	startByte int
	endByte   int
}

func (failure *boundedSourceSyntaxFailure) Error() string {
	if failure == nil {
		return "unknown parser failure at line 1 column 1"
	}
	return fmt.Sprintf(
		"%s at line %d column %d", failure.kind, failure.line, failure.column,
	)
}

func firstBoundedSourceSyntaxFailure(node *treesitter.Node) *boundedSourceSyntaxFailure {
	leaf := smallestNonemptyParserErrorLeaf(node)
	if leaf == nil {
		return nil
	}
	position := leaf.StartPosition()
	return &boundedSourceSyntaxFailure{
		kind: leaf.Kind(), line: int(position.Row) + 1, column: int(position.Column) + 1,
		startByte: int(leaf.StartByte()), endByte: int(leaf.EndByte()),
	}
}

// smallestNonemptyParserErrorLeaf returns only a byte range the parser proves
// contains no accepted surrounding syntax. Missing nodes are zero-width repair
// suggestions and cannot authorize model mutation. A composite ERROR node is
// also insufficient unless it contains a smaller exact error leaf. Tree-sitter
// may wrap one unexpected token in an ERROR node whose sole child covers the
// identical range; that wrapper remains one exact contiguous leaf.
func smallestNonemptyParserErrorLeaf(node *treesitter.Node) *treesitter.Node {
	if node == nil || (!node.HasError() && !node.IsError() && !node.IsMissing()) {
		return nil
	}
	for index := uint(0); index < node.ChildCount(); index++ {
		child := node.Child(index)
		if child == nil || (!child.HasError() && !child.IsError() && !child.IsMissing()) {
			continue
		}
		if leaf := smallestNonemptyParserErrorLeaf(child); leaf != nil {
			return leaf
		}
	}
	if node.IsMissing() || !node.IsError() || node.StartByte() >= node.EndByte() {
		return nil
	}
	if node.ChildCount() == 0 {
		return node
	}
	if node.ChildCount() != 1 {
		return nil
	}
	child := node.Child(0)
	if child == nil || child.HasError() || child.IsMissing() ||
		child.StartByte() != node.StartByte() || child.EndByte() != node.EndByte() {
		return nil
	}
	return node
}

func canonicalBoundedSourceNode(node *treesitter.Node, skippedID uintptr, source []byte) string {
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
			output.WriteString(canonicalBoundedSourceNode(node.Child(index), skippedID, source))
		}
	}
	output.WriteByte(')')
	return output.String()
}
