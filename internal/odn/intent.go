package odn

import (
	"regexp"
	"sort"
	"strings"
)

var (
	actionVerbRe      = regexp.MustCompile(`\b(make|create|build|scaffold|generate|run|fix|write|add|remove|delete|edit|update|install|start|stop|test)\b`)
	targetScopeRe     = regexp.MustCompile(`\b(here|project|file|files|folder|directory|go|golang|html|server|tests?|workspace)\b`)
	conversationCueRe = regexp.MustCompile(`\b(why|how|what|thoughts|explain|architecture|design|strategy|should)\b`)
)

func ClassifyIntent(message string) IntentResult {
	normalized := strings.ToLower(strings.TrimSpace(message))
	if normalized == "" {
		return IntentResult{
			Classification: IntentAmbiguous,
			Confidence:     0,
			ReasonCodes:    []string{"empty_message"},
		}
	}

	hasActionVerb := actionVerbRe.MatchString(normalized)
	hasTargetScope := targetScopeRe.MatchString(normalized)
	hasConversationCue := conversationCueRe.MatchString(normalized)
	hasQuestionMark := strings.Contains(normalized, "?")

	reasons := map[string]struct{}{}
	class := IntentAmbiguous
	confidence := 0.55

	if hasActionVerb {
		reasons["action_verb_detected"] = struct{}{}
	}
	if hasTargetScope {
		reasons["target_scope_detected"] = struct{}{}
	}
	if hasConversationCue {
		reasons["conversation_cue_detected"] = struct{}{}
	}
	if hasQuestionMark {
		reasons["question_mark_detected"] = struct{}{}
	}

	switch {
	case hasActionVerb && hasTargetScope && !hasQuestionMark:
		class = IntentExecution
		confidence = 0.92
	case hasActionVerb && hasTargetScope && hasQuestionMark:
		class = IntentAmbiguous
		confidence = 0.61
	case hasActionVerb && !hasTargetScope:
		class = IntentAmbiguous
		confidence = 0.66
	case !hasActionVerb && (hasConversationCue || hasQuestionMark):
		class = IntentConversation
		confidence = 0.86
	default:
		class = IntentConversation
		confidence = 0.72
		reasons["default_conversation_fallback"] = struct{}{}
	}

	if confidence < intentConfidenceThreshold {
		class = IntentAmbiguous
		reasons["low_confidence_threshold"] = struct{}{}
	}

	orderedReasons := make([]string, 0, len(reasons))
	for reason := range reasons {
		orderedReasons = append(orderedReasons, reason)
	}
	sort.Strings(orderedReasons)

	return IntentResult{
		Classification: class,
		Confidence:     confidence,
		ReasonCodes:    orderedReasons,
	}
}
