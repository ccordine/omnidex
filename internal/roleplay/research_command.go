package roleplay

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxResearchQuestionBytes = 1024

const researchCommandPrefix = "/research"

type ResearchCommand struct {
	Exact    string
	Question string
}

// ParseResearchCommand recognizes the one reserved roleplay research grammar.
// Matched is true for malformed input inside the reserved /research namespace
// so callers fail it instead of routing it to another slash-command parser.
func ParseResearchCommand(exact string) (command ResearchCommand, matched bool, err error) {
	if exact != researchCommandPrefix && !strings.HasPrefix(exact, researchCommandPrefix+" ") {
		return ResearchCommand{}, false, nil
	}
	matched = true
	literal := strings.TrimPrefix(exact, researchCommandPrefix+" ")
	if exact == researchCommandPrefix || literal == "" {
		return ResearchCommand{}, true, fmt.Errorf("research command must match /research \"question\"")
	}
	question, unquoteErr := strconv.Unquote(literal)
	if unquoteErr != nil || strconv.Quote(question) != literal {
		return ResearchCommand{}, true, fmt.Errorf("research command must contain one canonical quoted question")
	}
	if question == "" || question != strings.TrimSpace(question) ||
		len(question) > MaxResearchQuestionBytes || !utf8.ValidString(question) ||
		strings.ContainsAny(question, "\r\n") || strings.ContainsRune(question, '\x00') {
		return ResearchCommand{}, true, fmt.Errorf(
			"research question must be 1 to %d trimmed single-line UTF-8 bytes", MaxResearchQuestionBytes,
		)
	}
	return ResearchCommand{Exact: exact, Question: question}, true, nil
}
