package modelcontext

import "strings"

// PathIdentity is one byte-exact path-shaped span in model-visible text. Value
// is the code-owned artifact path when exact provenance resolves a bare name;
// otherwise it is the qualified path exactly as written.
type PathIdentity struct {
	Start int
	End   int
	Value string
}

// LexicalPathToken is one uninterpreted path-grammar token. It grants no
// artifact identity: callers must validate the complete token through one
// code-owned artifact adapter before it can become authority.
type LexicalPathToken struct {
	Start int
	End   int
	Value string
}

// LexicalPathTokens exposes the same deterministic tokenization used by the
// path-identity boundary without inferring that a dotted atom is a file. This
// lets an adapter registry recognize an explicitly written new artifact before
// that artifact exists in repository provenance.
func LexicalPathTokens(value string) []LexicalPathToken {
	spans := lexPathTokens(value)
	tokens := make([]LexicalPathToken, 0, len(spans))
	for _, span := range spans {
		tokens = append(tokens, LexicalPathToken{
			Start: span.start, End: span.end, Value: value[span.start:span.end],
		})
	}
	return tokens
}

// PathIdentities lexes cross-platform qualified paths and exact bare artifact
// names established by provenance. It does not infer file identity from a
// suffix, capitalization, repository convention, or product vocabulary.
func PathIdentities(value string, provenance ArtifactIdentityProvenance) []PathIdentity {
	identities := provenance.identities(value)
	spans := lexPathTokens(value)
	for _, span := range spans {
		if pathSpanOverlapsIdentity(span, identities) {
			continue
		}
		token := value[span.start:span.end]
		resolved, proven := provenance.resolve(token)
		if !proven && !isQualifiedPath(token) {
			continue
		}
		if !proven {
			resolved = token
		}
		identities = append(identities, PathIdentity{
			Start: span.start, End: span.end, Value: resolved,
		})
	}
	sortPathIdentities(identities)
	return identities
}

// ContainsPathIdentity reports only qualified paths. Bare dotted atoms such as
// Node.js, Vue.js, and http.Client are semantic text without exact artifact
// provenance and therefore remain visible.
func ContainsPathIdentity(value string) bool {
	return len(PathIdentities(value, ArtifactIdentityProvenance{})) > 0
}

// ContainsPathIdentityWithProvenance additionally recognizes exact known
// artifact paths and their unambiguous basenames.
func ContainsPathIdentityWithProvenance(
	value string,
	provenance ArtifactIdentityProvenance,
) bool {
	return len(PathIdentities(value, provenance)) > 0
}

// ProvenArtifactIdentities reports only exact current-tree identities. It is
// used at mixed typed prompts where route or media fields may legitimately be
// slash-shaped but known repository artifacts must still remain hidden.
func ProvenArtifactIdentities(
	value string,
	provenance ArtifactIdentityProvenance,
) []PathIdentity {
	identities := provenance.identities(value)
	sortPathIdentities(identities)
	return identities
}

func isQualifiedPath(value string) bool {
	if value == "" {
		return false
	}
	if value == "./" || value == "../" || strings.HasPrefix(value, "./") ||
		strings.HasPrefix(value, "../") {
		return true
	}
	if value[0] == '/' || value[0] == '\\' {
		return true
	}
	if value[0] == '~' {
		return true
	}
	if len(value) >= 2 && isASCIIAlpha(value[0]) && value[1] == ':' {
		return true
	}
	return strings.ContainsAny(value, `/\`)
}

func pathSpanOverlapsIdentity(span pathTokenSpan, identities []PathIdentity) bool {
	for _, identity := range identities {
		if span.start < identity.End && span.end > identity.Start {
			return true
		}
	}
	return false
}

func sortPathIdentities(identities []PathIdentity) {
	for index := 1; index < len(identities); index++ {
		for cursor := index; cursor > 0 && identities[cursor].Start < identities[cursor-1].Start; cursor-- {
			identities[cursor], identities[cursor-1] = identities[cursor-1], identities[cursor]
		}
	}
}

func isASCIIAlpha(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

type pathTokenSpan struct {
	start int
	end   int
}

func lexPathTokens(value string) []pathTokenSpan {
	spans := make([]pathTokenSpan, 0)
	for offset := 0; offset < len(value); {
		for offset < len(value) && pathTokenSeparator(value[offset]) {
			offset++
		}
		if offset >= len(value) {
			break
		}
		start, end := offset, offset
		if pathQuote(value[offset]) {
			quote := value[offset]
			start = offset + 1
			end = start
			for end < len(value) && value[end] != quote {
				end++
			}
			offset = end
			if offset < len(value) {
				offset++
			}
		} else {
			for end < len(value) && !pathTokenSeparator(value[end]) {
				end++
			}
			offset = end
		}
		start, end = trimPathToken(value, start, end)
		if start < end {
			spans = append(spans, pathTokenSpan{start: start, end: end})
		}
	}
	return spans
}

func pathTokenSeparator(value byte) bool {
	return value <= ' ' || value == ',' || value == ';'
}

func pathQuote(value byte) bool {
	return value == '\'' || value == '"' || value == '`'
}

func trimPathToken(value string, start, end int) (int, int) {
	for start < end && strings.ContainsRune("([{<", rune(value[start])) {
		start++
	}
	for end > start && strings.ContainsRune("')]}>,.!?", rune(value[end-1])) {
		end--
	}
	return start, end
}
