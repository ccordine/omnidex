package assemblyline

import (
	"fmt"
	"strings"
)

// validateFragmentRepairGuidanceInstruction rejects parser-proven source
// declarations. Repair guidance owns one imperative transformation only; the
// separate correction station owns replacement source. Signature text by
// itself remains ordinary instruction content and carries no source shape.
func validateFragmentRepairGuidanceInstruction(
	input FragmentRepairGuidanceInput,
	instruction string,
) error {
	if strings.TrimSpace(input.Signature) == "" {
		return fmt.Errorf("fragment repair-guidance shape requires one exact signature")
	}
	containsDeclaration, err := fragmentRepairGuidanceContainsSourceDeclaration(
		input.Language, instruction,
	)
	if err != nil {
		return fmt.Errorf("inspect fragment repair-guidance source shape: %w", err)
	}
	if containsDeclaration {
		return fmt.Errorf(
			"fragment repair guidance contains a complete source declaration; replacement source is forbidden",
		)
	}
	containsBody, err := fragmentRepairGuidanceContainsSourceOnlyBody(
		input.Language, instruction,
	)
	if err != nil {
		return fmt.Errorf("inspect fragment repair-guidance source block: %w", err)
	}
	if containsBody {
		return fmt.Errorf(
			"fragment repair guidance is a complete source-only block; an imperative instruction is required",
		)
	}
	return nil
}

func fragmentRepairGuidanceContainsSourceOnlyBody(
	languageID string,
	instruction string,
) (bool, error) {
	candidates := fragmentRepairGuidanceSourceBodyCandidates(instruction)
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		var contains bool
		var err error
		switch languageID {
		case "go":
			contains = fragmentRepairGuidanceIsWholeGoSourceBody(candidate)
		case "typescript":
			contains, err = fragmentRepairGuidanceIsWholeTypeScriptSourceBody(candidate)
		default:
			language, languageErr := boundedSourceLanguageByID(languageID)
			if languageErr != nil {
				return false, languageErr
			}
			contains, err = fragmentRepairGuidanceIsWholeBoundedSourceBody(language, candidate)
		}
		if err != nil || contains {
			return contains, err
		}
	}
	return false, nil
}

func fragmentRepairGuidanceSourceBodyCandidates(instruction string) []string {
	candidates := make([]string, 0, 4)
	seen := make(map[string]struct{})
	appendCandidate := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		if _, exists := seen[candidate]; exists {
			return
		}
		seen[candidate] = struct{}{}
		candidates = append(candidates, candidate)
	}
	appendCandidate(instruction)
	for offset := 0; offset < len(instruction); offset++ {
		if instruction[offset] == '\n' {
			appendCandidate(instruction[offset+1:])
		}
	}
	for _, fenced := range fragmentRepairGuidanceFencedContents(instruction) {
		appendCandidate(fenced)
	}
	return candidates
}

// fragmentRepairGuidanceFencedContents returns only explicit Markdown fenced
// block bodies. The first line after an opening fence is an information string
// owned by Markdown syntax, not source. Unterminated fences remain ordinary
// prose and receive no special parsing authority.
func fragmentRepairGuidanceFencedContents(value string) []string {
	contents := make([]string, 0, 1)
	for offset := 0; offset < len(value); {
		start := strings.Index(value[offset:], "```")
		if start < 0 {
			break
		}
		start += offset
		openingEnd := start
		for openingEnd < len(value) && value[openingEnd] == '`' {
			openingEnd++
		}
		lineEnd := strings.IndexByte(value[openingEnd:], '\n')
		if lineEnd < 0 {
			break
		}
		contentStart := openingEnd + lineEnd + 1
		closing := strings.Index(value[contentStart:], "```")
		if closing < 0 {
			break
		}
		closing += contentStart
		contents = append(contents, strings.TrimSpace(value[contentStart:closing]))
		closingEnd := closing
		for closingEnd < len(value) && value[closingEnd] == '`' {
			closingEnd++
		}
		offset = closingEnd
	}
	return contents
}

func fragmentRepairGuidanceContainsSourceDeclaration(
	languageID string,
	instruction string,
) (bool, error) {
	contains, err := fragmentRepairGuidanceContainsOneSourceDeclaration(
		languageID, fragmentRepairGuidanceFenceView(instruction),
	)
	if err != nil || contains {
		return contains, err
	}
	// Parse every balanced inline span independently. A complete declaration
	// remains replacement source even when Markdown or prose quotes surround
	// it; code cannot safely infer that declaration-shaped bytes are merely
	// user-visible display text.
	for _, span := range fragmentRepairGuidanceQuotedSpans(instruction) {
		raw := instruction[span.start+1 : span.end-1]
		views := []string{raw}
		decoded := fragmentRepairGuidanceInlineUnescapedView(
			raw, instruction[span.start],
		)
		if decoded != raw {
			views = append(views, decoded)
		}
		for _, view := range views {
			contains, err = fragmentRepairGuidanceContainsOneSourceDeclaration(
				languageID, view,
			)
			if err != nil || contains {
				return contains, err
			}
		}
	}
	return false, nil
}

// fragmentRepairGuidanceInlineUnescapedView removes exactly one presentation
// quote layer. Only an escaped matching delimiter and an escaped backslash are
// decoded; all language-level escapes remain untouched for the source parser.
func fragmentRepairGuidanceInlineUnescapedView(raw string, quote byte) string {
	var view strings.Builder
	view.Grow(len(raw))
	for offset := 0; offset < len(raw); offset++ {
		if raw[offset] == '\\' && offset+1 < len(raw) &&
			(raw[offset+1] == quote || raw[offset+1] == '\\') {
			offset++
		}
		view.WriteByte(raw[offset])
	}
	return view.String()
}

func fragmentRepairGuidanceContainsOneSourceDeclaration(
	languageID string,
	source string,
) (bool, error) {
	switch languageID {
	case "go":
		return fragmentRepairGuidanceContainsGoDeclaration(source), nil
	case "typescript":
		return fragmentRepairGuidanceContainsTypeScriptDeclaration(source)
	default:
		language, err := boundedSourceLanguageByID(languageID)
		if err != nil {
			return false, err
		}
		return fragmentRepairGuidanceContainsBoundedDeclaration(language, source)
	}
}

type fragmentRepairGuidanceQuotedSpan struct {
	start int
	end   int
}

// fragmentRepairGuidanceQuotedSpans identifies balanced prose, literal, and
// inline-code spans so each span can be checked independently for a forbidden
// complete declaration. Triple-backtick fences are handled by the full-view
// parser and therefore are not returned as inline spans.
func fragmentRepairGuidanceQuotedSpans(instruction string) []fragmentRepairGuidanceQuotedSpan {
	spans := make([]fragmentRepairGuidanceQuotedSpan, 0, 2)
	for offset := 0; offset < len(instruction); offset++ {
		quote := instruction[offset]
		if quote != '\'' && quote != '"' && quote != '`' ||
			quote == '`' && fragmentRepairGuidanceBacktickFenceByte(instruction, offset) ||
			!fragmentRepairGuidanceQuoteStart(instruction, offset) {
			continue
		}
		end, closed := fragmentRepairGuidanceQuoteEnd(instruction, offset+1, quote)
		if !closed {
			continue
		}
		spans = append(spans, fragmentRepairGuidanceQuotedSpan{start: offset, end: end + 1})
		offset = end
	}
	return spans
}

func fragmentRepairGuidanceBacktickFenceByte(value string, offset int) bool {
	start := offset
	for start > 0 && value[start-1] == '`' {
		start--
	}
	end := offset + 1
	for end < len(value) && value[end] == '`' {
		end++
	}
	return end-start >= 3
}

func fragmentRepairGuidanceFenceView(value string) string {
	view := []byte(value)
	for offset := 0; offset < len(view); {
		if view[offset] != '`' {
			offset++
			continue
		}
		end := offset + 1
		for end < len(view) && view[end] == '`' {
			end++
		}
		if end-offset >= 3 {
			for index := offset; index < end; index++ {
				view[index] = ' '
			}
		}
		offset = end
	}
	return string(view)
}

func fragmentRepairGuidanceQuoteStart(value string, offset int) bool {
	if value[offset] != '\'' || offset == 0 || offset+1 >= len(value) {
		return true
	}
	return !fragmentRepairGuidanceWordByte(value[offset-1]) ||
		!fragmentRepairGuidanceWordByte(value[offset+1])
}

func fragmentRepairGuidanceQuoteEnd(value string, start int, quote byte) (int, bool) {
	for offset := start; offset < len(value); offset++ {
		if value[offset] != quote {
			continue
		}
		escapes := 0
		for cursor := offset; cursor > start && value[cursor-1] == '\\'; cursor-- {
			escapes++
		}
		if escapes%2 == 0 {
			return offset, true
		}
	}
	return len(value), false
}

func fragmentRepairGuidanceWordByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' || value == '_'
}
