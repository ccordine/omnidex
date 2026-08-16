package retrieval

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxLexicalSearchAnchors = 3
	maxLexicalSearchTokens  = 24
)

// BuildLexicalAnchorQuery converts bounded semantic search anchors into one
// code-owned disjunctive lexical query. It only performs identifier lexical
// normalization; it does not interpret the request or select repository work.
func BuildLexicalAnchorQuery(anchors []string) (string, error) {
	if len(anchors) < 1 || len(anchors) > maxLexicalSearchAnchors {
		return "", fmt.Errorf("repository lexical search requires 1-%d anchors", maxLexicalSearchAnchors)
	}
	tokens := make([]string, 0, len(anchors)*2)
	seen := make(map[string]struct{})
	for _, anchor := range anchors {
		if anchor == "" || anchor != strings.TrimSpace(anchor) ||
			len([]byte(anchor)) > 256 || !utf8.ValidString(anchor) ||
			strings.ContainsRune(anchor, '\x00') {
			return "", fmt.Errorf("repository lexical search received an invalid anchor")
		}
		for _, token := range lexicalIdentifierTokens(anchor) {
			if _, duplicate := seen[token]; duplicate {
				continue
			}
			seen[token] = struct{}{}
			tokens = append(tokens, token)
			if len(tokens) > maxLexicalSearchTokens {
				return "", fmt.Errorf("repository lexical search exceeds %d unique tokens", maxLexicalSearchTokens)
			}
		}
	}
	if len(tokens) == 0 {
		return "", fmt.Errorf("repository lexical search anchors contain no identifier tokens")
	}
	terms := make([]string, len(tokens))
	for index, token := range tokens {
		terms[index] = `"` + token + `"`
	}
	query := strings.Join(terms, " OR ")
	if err := validateRetrievalQuery(query); err != nil {
		return "", err
	}
	return query, nil
}

func lexicalIdentifierTokens(value string) []string {
	runes := []rune(value)
	tokens := make([]string, 0, 4)
	current := make([]rune, 0, len(runes))
	flush := func() {
		if len(current) == 0 {
			return
		}
		tokens = append(tokens, strings.ToLower(string(current)))
		current = current[:0]
	}
	for index, currentRune := range runes {
		if !unicode.IsLetter(currentRune) && !unicode.IsDigit(currentRune) {
			flush()
			continue
		}
		if len(current) > 0 && unicode.IsUpper(currentRune) {
			previousRune := runes[index-1]
			nextIsLower := index+1 < len(runes) && unicode.IsLower(runes[index+1])
			if unicode.IsLower(previousRune) || unicode.IsDigit(previousRune) ||
				(unicode.IsUpper(previousRune) && nextIsLower) {
				flush()
			}
		}
		current = append(current, currentRune)
	}
	flush()
	return tokens
}
