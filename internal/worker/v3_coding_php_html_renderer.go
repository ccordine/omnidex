package worker

import (
	"fmt"
	"regexp"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
	phpgrammar "github.com/tree-sitter/tree-sitter-php/bindings/go"
)

var phpHTMLRendererSignature = regexp.MustCompile(
	`^function renderFeature[0-9]{3}HTML\(TaskResult \$result\): string$`,
)

var phpHTMLExecutableAttribute = regexp.MustCompile(`(?i)<[a-z][^>]*\son[a-z]+\s*=`)

func validatePHPHTMLRenderer(source []byte) error {
	parser := treesitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(treesitter.NewLanguage(phpgrammar.LanguagePHPOnly())); err != nil {
		return err
	}
	tree := parser.Parse(source, nil)
	if tree == nil {
		return fmt.Errorf("PHP HTML renderer parser returned no tree")
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil || root.HasError() || root.NamedChildCount() != 1 {
		return fmt.Errorf("PHP HTML renderer requires one parseable function")
	}
	function := root.NamedChild(0)
	body := function.ChildByFieldName("body")
	if function.Kind() != "function_definition" || body == nil || body.NamedChildCount() == 0 {
		return fmt.Errorf("PHP HTML renderer body must contain bounded rendering statements")
	}
	representation := newPHPHTMLRepresentationFlow(source)
	markup, err := representation.consumeBody(body)
	if err != nil {
		return err
	}
	if representation.fields == 0 {
		return fmt.Errorf("PHP HTML renderer must include one escaped TaskResult field")
	}
	lower := strings.ToLower(markup)
	for _, required := range []string{"<main", "</main>", "<h1", "</h1>", "class="} {
		if !strings.Contains(lower, required) {
			return fmt.Errorf("PHP HTML renderer body lacks required structural marker %s", required)
		}
	}
	if !strings.Contains(markup, "sm:") && !strings.Contains(markup, "md:") &&
		!strings.Contains(markup, "lg:") && !strings.Contains(markup, "xl:") {
		return fmt.Errorf("PHP HTML renderer body lacks one responsive Tailwind utility")
	}
	if strings.Contains(lower, "<script") || strings.Contains(lower, "javascript:") ||
		phpHTMLExecutableAttribute.MatchString(markup) {
		return fmt.Errorf("PHP HTML renderer contains executable browser content")
	}
	if err := validateHTMLArtifactSource("representation.html", []byte(markup)); err != nil {
		return fmt.Errorf("PHP HTML renderer body: %w", err)
	}
	return nil
}

func phpExactScopedCall(source []byte, node *treesitter.Node, scope, method string) bool {
	if node == nil || node.Kind() != "scoped_call_expression" {
		return false
	}
	return phpNodeText(source, node.ChildByFieldName("scope")) == scope &&
		phpNodeText(source, node.ChildByFieldName("name")) == method
}

func phpExactResultField(source []byte, node *treesitter.Node, variable string) (string, bool) {
	node = phpAcceptanceUnwrapParentheses(node)
	if node == nil || node.Kind() != "member_access_expression" {
		return "", false
	}
	object, name := node.ChildByFieldName("object"), node.ChildByFieldName("name")
	if object == nil || object.Kind() != "variable_name" || phpNodeText(source, object) != variable ||
		name == nil || name.Kind() != "name" {
		return "", false
	}
	return phpNodeText(source, name), true
}

func phpHTMLSingleQuotedLiteral(source []byte, node *treesitter.Node) (string, error) {
	if node == nil || node.Kind() != "string" {
		return "", fmt.Errorf("PHP HTML representation requires single-quoted literals")
	}
	value := phpNodeText(source, node)
	if len(value) < 2 || value[0] != '\'' || value[len(value)-1] != '\'' {
		return "", fmt.Errorf("PHP HTML representation requires single-quoted literals")
	}
	value = value[1 : len(value)-1]
	var decoded strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '\\' {
			decoded.WriteByte(value[index])
			continue
		}
		if index+1 >= len(value) || value[index+1] != '\\' && value[index+1] != '\'' {
			return "", fmt.Errorf("PHP HTML representation contains an unsupported string escape")
		}
		index++
		decoded.WriteByte(value[index])
	}
	return decoded.String(), nil
}
