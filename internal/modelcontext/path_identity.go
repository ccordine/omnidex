package modelcontext

import (
	"regexp"
	"strings"
)

var bareFileNamePattern = regexp.MustCompile(
	`(?i)(?:^|[^[:alnum:]_])(?:[[:alnum:]_-][[:alnum:]_.-]*\.[a-z][a-z\d_-]{1,15}|\.[a-z][a-z\d_-]{1,15})(?:$|[^[:alnum:]_.-])`,
)

// ContainsPathIdentity rejects both qualified paths and ambiguous filename-like
// tokens before text enters a model-visible envelope. Losing prose is safer
// than disclosing repository or host identity.
func ContainsPathIdentity(value string) bool {
	if bareFileNamePattern.MatchString(value) {
		return true
	}
	for _, field := range strings.Fields(value) {
		token := strings.Trim(field, `"'()[]{}<>,;:`)
		if token == "" {
			continue
		}
		if strings.HasPrefix(token, "/") || strings.HasPrefix(token, "./") ||
			strings.HasPrefix(token, "../") {
			return true
		}
		if len(token) >= 3 && token[1] == ':' &&
			(token[2] == '/' || token[2] == '\\') {
			return true
		}
		if strings.ContainsAny(token, `/\`) && strings.IndexFunc(token, isASCIIAlpha) >= 0 {
			return true
		}
	}
	return false
}

func isASCIIAlpha(value rune) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}
