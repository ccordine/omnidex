package modelcontext

// RepairInstructionPathIdentities inspects mixed imperative prose and quoted
// source literals. Ordinary prose retains the strict qualified-path grammar.
// Only bytes inside a balanced source quote receive source-escape decoding, so
// a quoted line-break escape is semantic text while an escaped or encoded
// filesystem separator remains an identity.
func RepairInstructionPathIdentities(
	value string,
	provenance ArtifactIdentityProvenance,
) []PathIdentity {
	identities := make([]PathIdentity, 0)
	segmentStart := 0
	for offset := 0; offset < len(value); offset++ {
		if !pathQuote(value[offset]) || !repairInstructionQuoteStart(value, offset) {
			continue
		}
		end, closed := repairInstructionQuotedEnd(value, offset+1, value[offset])
		if !closed {
			continue
		}
		identities = appendAdjustedPathIdentities(
			identities, value[segmentStart:offset], segmentStart, provenance,
		)
		if repairInstructionRawQuote(value, offset) {
			// Backticks, single quotes, and Rust-style raw-string quotes have
			// language-dependent escape rules. This language-neutral boundary
			// preserves their backslashes instead of guessing an interpreted
			// dialect. Double-quoted ordinary literals retain decoded escapes.
			identities = appendAdjustedPathIdentities(
				identities, value[offset+1:end], offset+1, provenance,
			)
		} else {
			identities = appendAdjustedSourceLiteralPathIdentities(
				identities, value[offset+1:end], offset+1, provenance,
			)
		}
		segmentStart = end + 1
		offset = end
	}
	identities = appendAdjustedPathIdentities(
		identities, value[segmentStart:], segmentStart, provenance,
	)
	sortPathIdentities(identities)
	return identities
}

func repairInstructionRawQuote(value string, offset int) bool {
	if value[offset] == '`' || value[offset] == '\'' {
		return true
	}
	if value[offset] != '"' || offset == 0 {
		return false
	}
	cursor := offset - 1
	for cursor >= 0 && value[cursor] == '#' {
		cursor--
	}
	if cursor < 0 || value[cursor] != 'r' {
		return false
	}
	start := cursor
	if start > 0 && (value[start-1] == 'b' || value[start-1] == 'c') {
		start--
	}
	return start == 0 || !repairInstructionWordByte(value[start-1])
}

func repairInstructionQuoteStart(value string, offset int) bool {
	if value[offset] != '\'' || offset == 0 || offset+1 >= len(value) {
		return true
	}
	return !repairInstructionWordByte(value[offset-1]) ||
		!repairInstructionWordByte(value[offset+1])
}

func repairInstructionQuotedEnd(value string, start int, quote byte) (int, bool) {
	for offset := start; offset < len(value); offset++ {
		if value[offset] == quote && !sourceByteEscaped(value, offset) {
			return offset, true
		}
	}
	return len(value), false
}

func repairInstructionWordByte(value byte) bool {
	return isASCIIAlpha(value) || value >= '0' && value <= '9' || value == '_'
}
