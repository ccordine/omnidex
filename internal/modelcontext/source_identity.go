package modelcontext

// SourcePathIdentities inspects only string-like literals and comments inside
// parser-proven source. Operators and regular-expression syntax remain source
// grammar, while path-bearing literals and module specifiers remain forbidden
// model context.
func SourcePathIdentities(
	source string,
	provenance ArtifactIdentityProvenance,
) []PathIdentity {
	identities := make([]PathIdentity, 0)
	for offset := 0; offset < len(source); {
		switch {
		case source[offset] == '/' && sourceRegularExpressionStart(source, offset):
			offset = sourceRegularExpressionEnd(source, offset+1)
		case source[offset] == '`':
			var end int
			identities, end = appendSourceTemplatePathIdentities(
				identities, source, offset+1, provenance,
			)
			offset = end
			if offset < len(source) {
				offset++
			}
		case source[offset] == '\'' || source[offset] == '"':
			quote := source[offset]
			start := offset + 1
			end := sourceQuotedEnd(source, start, quote)
			identities = appendAdjustedSourceLiteralPathIdentities(
				identities, source[start:end], start, provenance,
			)
			offset = end
			if offset < len(source) {
				offset++
			}
		case offset+1 < len(source) && source[offset:offset+2] == "//":
			start := offset + 2
			end := start
			for end < len(source) && source[end] != '\n' && source[end] != '\r' {
				end++
			}
			identities = appendAdjustedPathIdentities(
				identities, source[start:end], start, provenance,
			)
			offset = end
		case offset+1 < len(source) && source[offset:offset+2] == "/*":
			start := offset + 2
			end := start
			for end+1 < len(source) && source[end:end+2] != "*/" {
				end++
			}
			identities = appendAdjustedPathIdentities(
				identities, source[start:end], start, provenance,
			)
			if end+1 < len(source) {
				offset = end + 2
			} else {
				offset = len(source)
			}
		default:
			offset++
		}
	}
	return identities
}

func appendSourceTemplatePathIdentities(
	destination []PathIdentity,
	source string,
	start int,
	provenance ArtifactIdentityProvenance,
) ([]PathIdentity, int) {
	literalStart := start
	literalOnly := true
	for offset := start; offset < len(source); offset++ {
		switch {
		case source[offset] == '`' && !sourceByteEscaped(source, offset):
			if literalOnly {
				destination = appendAdjustedSourceLiteralPathIdentities(
					destination, source[literalStart:offset], literalStart, provenance,
				)
			} else {
				destination = appendAdjustedPathIdentities(
					destination, source[literalStart:offset], literalStart, provenance,
				)
			}
			return destination, offset
		case offset+1 < len(source) && source[offset:offset+2] == "${" &&
			!sourceByteEscaped(source, offset):
			literalOnly = false
			destination = appendAdjustedPathIdentities(
				destination, source[literalStart:offset], literalStart, provenance,
			)
			expressionStart := offset + 2
			expressionEnd := sourceTemplateExpressionEnd(source, expressionStart)
			destination = appendAdjustedSourcePathIdentities(
				destination, source[expressionStart:expressionEnd], expressionStart, provenance,
			)
			if expressionEnd >= len(source) {
				return destination, expressionEnd
			}
			offset = expressionEnd
			literalStart = expressionEnd + 1
		}
	}
	if literalOnly {
		destination = appendAdjustedSourceLiteralPathIdentities(
			destination, source[literalStart:], literalStart, provenance,
		)
	} else {
		destination = appendAdjustedPathIdentities(
			destination, source[literalStart:], literalStart, provenance,
		)
	}
	return destination, len(source)
}

func sourceTemplateExpressionEnd(source string, start int) int {
	depth := 1
	for offset := start; offset < len(source); {
		switch {
		case source[offset] == '\'' || source[offset] == '"':
			end := sourceQuotedEnd(source, offset+1, source[offset])
			offset = minSourceOffset(end+1, len(source))
		case source[offset] == '`':
			end := sourceQuotedEnd(source, offset+1, '`')
			offset = minSourceOffset(end+1, len(source))
		case offset+1 < len(source) && source[offset:offset+2] == "//":
			offset += 2
			for offset < len(source) && source[offset] != '\n' && source[offset] != '\r' {
				offset++
			}
		case offset+1 < len(source) && source[offset:offset+2] == "/*":
			offset += 2
			for offset+1 < len(source) && source[offset:offset+2] != "*/" {
				offset++
			}
			offset = minSourceOffset(offset+2, len(source))
		case source[offset] == '/' && sourceRegularExpressionStart(source, offset):
			offset = sourceRegularExpressionEnd(source, offset+1)
		case source[offset] == '{':
			depth++
			offset++
		case source[offset] == '}':
			depth--
			if depth == 0 {
				return offset
			}
			offset++
		default:
			offset++
		}
	}
	return len(source)
}

func sourceQuotedEnd(source string, start int, quote byte) int {
	for offset := start; offset < len(source); offset++ {
		if source[offset] != quote {
			continue
		}
		if !sourceByteEscaped(source, offset) {
			return offset
		}
	}
	return len(source)
}

func sourceRegularExpressionStart(source string, offset int) bool {
	if offset+1 >= len(source) || source[offset+1] == '/' || source[offset+1] == '*' {
		return false
	}
	previous := offset - 1
	for previous >= 0 && source[previous] <= ' ' {
		previous--
	}
	if previous < 0 || stringsContainsByte("=(:,[!&|?{};", source[previous]) {
		return true
	}
	end := previous + 1
	for previous >= 0 && isASCIIAlpha(source[previous]) {
		previous--
	}
	switch source[previous+1 : end] {
	case "return", "case", "throw":
		return true
	default:
		return false
	}
}

func sourceRegularExpressionEnd(source string, offset int) int {
	inClass := false
	for offset < len(source) {
		switch {
		case source[offset] == '\\':
			offset += 2
			continue
		case source[offset] == '[':
			inClass = true
		case source[offset] == ']':
			inClass = false
		case source[offset] == '/' && !inClass:
			offset++
			for offset < len(source) && isASCIIAlpha(source[offset]) {
				offset++
			}
			return offset
		case source[offset] == '\n' || source[offset] == '\r':
			return offset
		}
		offset++
	}
	return offset
}

func stringsContainsByte(values string, value byte) bool {
	for index := 0; index < len(values); index++ {
		if values[index] == value {
			return true
		}
	}
	return false
}

func sourceByteEscaped(source string, offset int) bool {
	escapes := 0
	for offset > 0 && source[offset-1] == '\\' {
		escapes++
		offset--
	}
	return escapes%2 == 1
}

func appendAdjustedPathIdentities(
	destination []PathIdentity,
	value string,
	offset int,
	provenance ArtifactIdentityProvenance,
) []PathIdentity {
	for _, identity := range PathIdentities(value, provenance) {
		identity.Start += offset
		identity.End += offset
		destination = append(destination, identity)
	}
	return destination
}

func appendAdjustedSourceLiteralPathIdentities(
	destination []PathIdentity,
	value string,
	offset int,
	provenance ArtifactIdentityProvenance,
) []PathIdentity {
	if value == "." || value == ".." {
		destination = append(destination, PathIdentity{
			Start: offset, End: offset + len(value), Value: value,
		})
		return destination
	}
	return appendAdjustedPathIdentities(destination, value, offset, provenance)
}

func appendAdjustedSourcePathIdentities(
	destination []PathIdentity,
	value string,
	offset int,
	provenance ArtifactIdentityProvenance,
) []PathIdentity {
	for _, identity := range SourcePathIdentities(value, provenance) {
		identity.Start += offset
		identity.End += offset
		destination = append(destination, identity)
	}
	return destination
}

func minSourceOffset(left, right int) int {
	if left < right {
		return left
	}
	return right
}
