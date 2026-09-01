package worker

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	treesitter "github.com/tree-sitter/go-tree-sitter"
)

func (validator *directCodingBrowserAcceptanceQueryValidator) validateOutputQueryCall(
	call *treesitter.Node,
	methodName string,
	method directCodingBrowserScreenQueryMethod,
	arguments *treesitter.Node,
) error {
	if method.plural {
		return fmt.Errorf(
			"browser acceptance status output query %s must be singular",
			methodName,
		)
	}
	if arguments == nil || arguments.NamedChildCount() != 2 {
		return fmt.Errorf(
			"browser acceptance status output query %s requires one exact receipt accessible name",
			methodName,
		)
	}
	name, hasName, err := validator.queryName(arguments)
	if err != nil || !hasName {
		return fmt.Errorf(
			"browser acceptance status output query %s requires one exact receipt accessible name",
			methodName,
		)
	}
	var matches []directCodingBrowserPublicOutput
	for _, output := range validator.surface.Outputs {
		if output.AccessibleName == name {
			matches = append(matches, output)
		}
	}
	if len(matches) != 1 {
		return fmt.Errorf(
			"browser acceptance public surface has no status output with accessible name %q",
			name,
		)
	}
	if method.asynchronous && !directCodingBrowserCallIsAwaited(call) {
		return fmt.Errorf(
			"browser acceptance status output query %s must be explicitly awaited",
			methodName,
		)
	}
	selection := call
	if method.asynchronous {
		selection = directCodingBrowserAwaitExpression(call)
	}
	validator.outputSelections[selection.Id()] = matches[0]
	return nil
}

func directCodingBrowserOutputOutcomeMatcher(
	matcher directCodingBrowserExpectationMatcher,
	validator *directCodingBrowserAcceptanceQueryValidator,
) bool {
	if matcher.negated || matcher.name != "toHaveTextContent" ||
		matcher.arguments == nil || matcher.arguments.NamedChildCount() != 1 {
		return false
	}
	_, valid := validator.exactAnchoredLiteralRegex(matcher.arguments.NamedChild(0))
	return valid
}

func (validator *directCodingBrowserAcceptanceQueryValidator) exactAnchoredLiteralRegex(
	node *treesitter.Node,
) (string, bool) {
	if node == nil || node.Kind() != "regex" || node.ChildByFieldName("flags") != nil {
		return "", false
	}
	patternNode := node.ChildByFieldName("pattern")
	if patternNode == nil || patternNode.Kind() != "regex_pattern" {
		return "", false
	}
	pattern := validator.text(patternNode)
	if len(pattern) < 2 || pattern[0] != '^' || pattern[len(pattern)-1] != '$' {
		return "", false
	}
	body := pattern[1 : len(pattern)-1]
	var literal strings.Builder
	literal.Grow(len(body))
	for index := 0; index < len(body); index++ {
		character := body[index]
		if character == '\\' {
			index++
			if index >= len(body) || !directCodingBrowserEscapedRegexLiteral(body[index]) {
				return "", false
			}
			literal.WriteByte(body[index])
			continue
		}
		if strings.ContainsRune(`.^$*+?()[]{}|/`, rune(character)) {
			return "", false
		}
		literal.WriteByte(character)
	}
	value := literal.String()
	if value == "" || !utf8.ValidString(value) ||
		strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", false
	}
	return value, true
}

func directCodingBrowserEscapedRegexLiteral(character byte) bool {
	return strings.ContainsRune(`\\.^$*+?()[]{}|/`, rune(character))
}
