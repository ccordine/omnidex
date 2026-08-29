package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

// canonicalizeDirectCodingTypeScriptDiagnosticRegularExpressions projects
// parser-proven source regexes into lossless path-free prose. Only complete
// diagnostic tokens are eligible; a regex-shaped prefix never authorizes a
// larger filesystem token.
func canonicalizeDirectCodingTypeScriptDiagnosticRegularExpressions(
	value string,
	literals []string,
) (string, error) {
	replacements := make(map[string]string, len(literals))
	for index, literal := range literals {
		canonical, err := canonicalDirectCodingTypeScriptRegularExpression(literal)
		if err != nil {
			return "", fmt.Errorf("regular expression %d: %w", index, err)
		}
		replacements[literal] = canonical
	}
	if len(replacements) == 0 {
		return value, nil
	}
	tokens := modelcontext.LexicalPathTokens(value)
	var projected strings.Builder
	previous := 0
	changed := false
	for _, token := range tokens {
		canonical, exists := replacements[token.Value]
		if !exists {
			continue
		}
		if token.Start < previous || token.End < token.Start || token.End > len(value) {
			return "", fmt.Errorf("regular-expression diagnostic token has invalid byte bounds")
		}
		projected.WriteString(value[previous:token.Start])
		projected.WriteString(canonical)
		previous = token.End
		changed = true
	}
	if !changed {
		return value, nil
	}
	projected.WriteString(value[previous:])
	return projected.String(), nil
}

func canonicalDirectCodingTypeScriptRegularExpression(literal string) (string, error) {
	parsed, err := assemblyline.TypeScriptRegularExpressionLiterals(literal, false)
	if err != nil {
		return "", fmt.Errorf("parse exact literal: %w", err)
	}
	if len(parsed) != 1 || parsed[0] != literal {
		return "", fmt.Errorf("authority must be exactly one parser-proven literal")
	}
	pattern, flags, err := splitDirectCodingTypeScriptRegularExpression(literal)
	if err != nil {
		return "", err
	}
	canonical := ""
	if directCodingRegularExpressionPlainSourceText(pattern, flags) {
		matching := "case-sensitive"
		if flags == "i" {
			matching = "case-insensitive"
		}
		canonical = `plain text "` + pattern + `" (` + matching + `)`
	} else {
		canonical = "regular expression pattern formed from ordered components [" +
			encodeDirectCodingRegularExpressionComponents(pattern) +
			"]. Read the ordered components as regular-expression source characters. Words and numeric labels describing a component are not pattern text. Active flags [" +
			describeDirectCodingRegularExpressionFlags(flags) + "]."
	}
	if modelcontext.ContainsPathIdentity(canonical) {
		return "", fmt.Errorf("canonical regular expression retained path identity")
	}
	return canonical, nil
}

// Plain source text is a compact lossless projection only when later
// whitespace normalization cannot change it and no byte has regular-expression
// operator meaning. All other patterns use the exact ordered-byte description.
func directCodingRegularExpressionPlainSourceText(pattern string, flags string) bool {
	if flags != "" && flags != "i" {
		return false
	}
	if pattern == "" || pattern[0] == ' ' || pattern[len(pattern)-1] == ' ' {
		return false
	}
	previousSpace := false
	for offset := 0; offset < len(pattern); offset++ {
		if directCodingRegularExpressionTextByte(pattern[offset]) {
			previousSpace = false
			continue
		}
		if pattern[offset] != ' ' || previousSpace {
			return false
		}
		previousSpace = true
	}
	return true
}

func splitDirectCodingTypeScriptRegularExpression(literal string) (string, string, error) {
	if len(literal) < 2 || literal[0] != '/' {
		return "", "", fmt.Errorf("literal must begin with a solidus")
	}
	inClass := false
	for offset := 1; offset < len(literal); offset++ {
		switch literal[offset] {
		case '\\':
			offset++
			if offset >= len(literal) {
				return "", "", fmt.Errorf("literal ends with an incomplete escape")
			}
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '/':
			if inClass {
				continue
			}
			flags := literal[offset+1:]
			for index := 0; index < len(flags); index++ {
				if flags[index] < 'A' || flags[index] > 'Z' && flags[index] < 'a' || flags[index] > 'z' {
					return "", "", fmt.Errorf("literal has a non-alphabetic flag byte")
				}
			}
			return literal[1:offset], flags, nil
		}
	}
	return "", "", fmt.Errorf("literal has no closing solidus")
}

func encodeDirectCodingRegularExpressionComponents(value string) string {
	if value == "" {
		return "empty pattern"
	}
	components := make([]string, 0, len(value))
	for offset := 0; offset < len(value); {
		if directCodingRegularExpressionTextByte(value[offset]) {
			end := offset + 1
			for end < len(value) && directCodingRegularExpressionTextByte(value[end]) {
				end++
			}
			components = append(components, `source text "`+value[offset:end]+`"`)
			offset = end
			continue
		}
		components = append(components, describeDirectCodingRegularExpressionByte(value[offset]))
		offset++
	}
	return strings.Join(components, "; ")
}

func describeDirectCodingRegularExpressionByte(value byte) string {
	names := map[byte]string{
		'\t': "horizontal-tab",
		' ':  "space",
		'!':  "exclamation-mark",
		'$':  "dollar-sign",
		'(':  "left-parenthesis",
		')':  "right-parenthesis",
		'*':  "asterisk",
		'+':  "plus-sign",
		'-':  "hyphen-minus",
		'.':  "full-stop",
		'/':  "forward-slash (solidus)",
		'?':  "question-mark",
		'[':  "left-square-bracket",
		'\\': "backslash (reverse solidus)",
		']':  "right-square-bracket",
		'^':  "circumflex-accent",
		'_':  "low-line",
		'{':  "left-curly-bracket",
		'|':  "vertical-line",
		'}':  "right-curly-bracket",
	}
	if name, exists := names[value]; exists {
		return fmt.Sprintf("one %s character (U+%04X)", name, value)
	}
	return fmt.Sprintf("one source byte hexadecimal %02X", value)
}

func describeDirectCodingRegularExpressionFlags(flags string) string {
	if flags == "" {
		return "none"
	}
	descriptions := make([]string, 0, len(flags))
	for offset := 0; offset < len(flags); offset++ {
		meaning := "registered ECMAScript flag"
		switch flags[offset] {
		case 'd':
			meaning = "produce match indices"
		case 'g':
			meaning = "global matching"
		case 'i':
			meaning = "case-insensitive matching"
		case 'm':
			meaning = "multiline anchors"
		case 's':
			meaning = "dot matches line terminators"
		case 'u':
			meaning = "Unicode matching"
		case 'v':
			meaning = "Unicode set matching"
		case 'y':
			meaning = "sticky matching"
		}
		descriptions = append(descriptions, fmt.Sprintf(
			`source flag "%c": %s`, flags[offset], meaning,
		))
	}
	return strings.Join(descriptions, "; ")
}

func directCodingRegularExpressionTextByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9'
}
