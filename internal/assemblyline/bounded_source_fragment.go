package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
	"unsafe"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func validateBoundedSourceFragment(
	language boundedSourceLanguage,
	signature string,
	candidate string,
) (string, error) {
	signature = strings.TrimSpace(signature)
	if signature == "" || strings.ContainsAny(signature, "\x00\r\n") ||
		!utf8.ValidString(signature) || len(signature) > 1024 {
		return "", fmt.Errorf("%s fragment signature must be one trimmed line", language.display)
	}
	content, actual, err := projectBoundedSourceDeclaration(language, candidate)
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

func projectBoundedSourceFragment(
	language boundedSourceLanguage,
	raw string,
) (PortableResultProjection, error) {
	content, _, err := projectBoundedSourceDeclaration(language, raw)
	if err != nil {
		return PortableResultProjection{}, err
	}
	startByte := strings.Index(raw, content)
	if startByte < 0 {
		return PortableResultProjection{}, fmt.Errorf(
			"%s declaration is not an exact response span", language.display,
		)
	}
	return NewSourceDeclarationPortableResultProjection(
		raw, content, startByte, startByte+len(content),
	)
}

func projectBoundedSourceDeclaration(
	language boundedSourceLanguage,
	candidate string,
) (string, string, error) {
	content := strings.TrimSpace(candidate)
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
		return nil, nil, fmt.Errorf("%s syntax rejected: %s", language.display, detail)
	}
	return parser, tree, nil
}

func firstBoundedSourceSyntaxFailure(node *treesitter.Node) string {
	if node == nil {
		return "unknown parser failure at line 1 column 1"
	}
	if node.IsError() || node.IsMissing() {
		position := node.StartPosition()
		return fmt.Sprintf(
			"%s at line %d column %d", node.Kind(), position.Row+1, position.Column+1,
		)
	}
	for index := uint(0); index < node.ChildCount(); index++ {
		child := node.Child(index)
		if child != nil && child.HasError() {
			return firstBoundedSourceSyntaxFailure(child)
		}
	}
	position := node.StartPosition()
	return fmt.Sprintf("invalid syntax at line %d column %d", position.Row+1, position.Column+1)
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
