package assemblyline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
	typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

func parseTypeScriptResponseTree(
	source string,
	tsx bool,
) (*treesitter.Parser, *treesitter.Tree, error) {
	parser := treesitter.NewParser()
	languagePointer := typescript.LanguageTypescript()
	if tsx {
		languagePointer = typescript.LanguageTSX()
	}
	if err := parser.SetLanguage(treesitter.NewLanguage(languagePointer)); err != nil {
		parser.Close()
		return nil, nil, fmt.Errorf("configure TypeScript response projector: %w", err)
	}
	tree := parser.Parse([]byte(source), nil)
	if tree == nil {
		parser.Close()
		return nil, nil, fmt.Errorf("TypeScript response projector returned no syntax tree")
	}
	return parser, tree, nil
}

func typeScriptResponseSegments(raw string, tsx bool) []typeScriptResponseSegment {
	segments := make([]typeScriptResponseSegment, 0, 3)
	cursor := 0
	outsideStart := 0
	fenceStart := -1
	fenceAccepted := false
	for cursor < len(raw) {
		lineStart := cursor
		lineEnd := strings.IndexByte(raw[cursor:], '\n')
		if lineEnd < 0 {
			cursor = len(raw)
		} else {
			cursor += lineEnd + 1
		}
		line := strings.TrimSpace(raw[lineStart:cursor])
		if fenceStart < 0 {
			language, opening := typeScriptFenceOpening(line)
			if !opening {
				continue
			}
			if outsideStart < lineStart {
				segments = append(segments, typeScriptResponseSegment{
					startByte: outsideStart, endByte: lineStart,
				})
			}
			fenceStart = cursor
			fenceAccepted = language == "" || language == "ts" || language == "typescript" ||
				(tsx && language == "tsx")
			continue
		}
		if line != "```" {
			continue
		}
		if fenceAccepted && fenceStart < lineStart {
			segments = append(segments, typeScriptResponseSegment{
				startByte: fenceStart, endByte: trimFenceBodyEnd(raw, fenceStart, lineStart), fenced: true,
			})
		}
		outsideStart = cursor
		fenceStart = -1
		fenceAccepted = false
	}
	if fenceStart >= 0 {
		if fenceAccepted && fenceStart < len(raw) {
			segments = append(segments, typeScriptResponseSegment{
				startByte: fenceStart, endByte: len(raw), fenced: true,
			})
		}
		outsideStart = len(raw)
	}
	if outsideStart < len(raw) {
		segments = append(segments, typeScriptResponseSegment{
			startByte: outsideStart, endByte: len(raw),
		})
	}
	if len(segments) == 0 {
		return []typeScriptResponseSegment{{startByte: 0, endByte: len(raw)}}
	}
	return segments
}

func typeScriptFenceOpening(line string) (string, bool) {
	if !strings.HasPrefix(line, "```") || line == "```" {
		return "", line == "```"
	}
	language := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "```")))
	if strings.ContainsAny(language, " `\t") {
		return "", false
	}
	return language, true
}

func trimFenceBodyEnd(raw string, start, end int) int {
	for end > start && (raw[end-1] == '\n' || raw[end-1] == '\r') {
		end--
	}
	return end
}

func collectTypeScriptResponseFunctions(
	node *treesitter.Node,
	source []byte,
	insideFunction bool,
	functions []typeScriptFunctionNode,
) []typeScriptFunctionNode {
	if node == nil {
		return functions
	}
	if node.Kind() == "function_declaration" || isRecoverableTopLevelFunctionExpression(node) {
		if insideFunction {
			return functions
		}
		name := node.ChildByFieldName("name")
		if name != nil {
			startByte, endByte := int(node.StartByte()), int(node.EndByte())
			exportWrapper := typeScriptExpandedAuthorityAncestor(node)
			if exportWrapper != nil {
				startByte, endByte = int(exportWrapper.StartByte()), int(exportWrapper.EndByte())
			}
			functions = append(functions, typeScriptFunctionNode{
				name: name.Utf8Text(source), startByte: startByte,
				endByte: endByte, exportWrap: exportWrapper != nil,
			})
		}
		return functions
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		functions = collectTypeScriptResponseFunctions(
			node.NamedChild(index), source, insideFunction, functions,
		)
	}
	return functions
}

func isRecoverableTopLevelFunctionExpression(node *treesitter.Node) bool {
	if node == nil || node.Kind() != "function_expression" {
		return false
	}
	statement := node.Parent()
	return statement != nil && statement.Kind() == "expression_statement" &&
		statement.Parent() != nil && statement.Parent().Kind() == "program"
}

func typeScriptExpandedAuthorityAncestor(node *treesitter.Node) *treesitter.Node {
	for ancestor := node.Parent(); ancestor != nil; ancestor = ancestor.Parent() {
		switch ancestor.Kind() {
		case "export_statement", "ambient_declaration":
			return ancestor
		case "program":
			return nil
		}
	}
	return nil
}

func firstExtraTypeScriptExecutableNode(
	node *treesitter.Node,
	selectedStart int,
	selectedEnd int,
) string {
	if node == nil {
		return ""
	}
	start, end := int(node.StartByte()), int(node.EndByte())
	if start == selectedStart && end == selectedEnd && node.Kind() == "function_declaration" {
		return ""
	}
	if end <= selectedStart || start >= selectedEnd {
		if unmistakableTypeScriptExecutableNode(node) {
			return node.Kind()
		}
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if kind := firstExtraTypeScriptExecutableNode(
			node.NamedChild(index), selectedStart, selectedEnd,
		); kind != "" {
			return kind
		}
	}
	return ""
}

func unmistakableTypeScriptExecutableNode(node *treesitter.Node) bool {
	if node == nil || node.HasError() || node.IsError() || node.IsMissing() {
		return false
	}
	switch node.Kind() {
	case "function_declaration", "class_declaration", "lexical_declaration",
		"variable_declaration", "import_statement", "export_statement",
		"enum_declaration", "interface_declaration", "type_alias_declaration",
		"ambient_declaration", "internal_module":
		return true
	case "expression_statement":
		return containsTypeScriptSideEffect(node)
	default:
		return false
	}
}

func containsTypeScriptSideEffect(node *treesitter.Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "call_expression", "assignment_expression", "augmented_assignment_expression",
		"update_expression", "new_expression", "await_expression", "yield_expression":
		return true
	}
	for index := uint(0); index < node.NamedChildCount(); index++ {
		if containsTypeScriptSideEffect(node.NamedChild(index)) {
			return true
		}
	}
	return false
}

func typeScriptProjectionSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
