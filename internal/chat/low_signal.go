package chat

import (
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

var lowSignalTokens = map[string]struct{}{
	"hi": {}, "hello": {}, "hey": {}, "yo": {}, "sup": {},
	"ping": {}, "test": {}, "testing": {}, "check": {}, "checking": {},
}

// IsLowSignal reports brief conversational check-ins that should not run the full pipeline.
func IsLowSignal(instruction, pipeline string) bool {
	if strings.ToLower(strings.TrimSpace(pipeline)) != model.PipelineChat {
		return false
	}

	value := strings.ToLower(strings.TrimSpace(instruction))
	if value == "" {
		return false
	}
	value = strings.Trim(value, "\"'`.,!?;:()[]{}")
	if value == "" {
		return false
	}

	words := strings.Fields(value)
	if len(words) == 0 || len(words) > 3 {
		return false
	}
	if _, ok := lowSignalTokens[value]; ok {
		return true
	}
	if _, ok := lowSignalTokens[words[0]]; ok {
		return true
	}
	return false
}

// LowSignalResponse returns a direct greeting for brief chat check-ins.
func LowSignalResponse(instruction string) string {
	value := strings.ToLower(strings.Trim(strings.TrimSpace(instruction), "\"'`.,!?;:()[]{}"))
	switch value {
	case "hi", "hello", "hey", "yo", "sup":
		return "Hi! How can I help you today?"
	case "ping", "test", "testing", "check", "checking":
		return "I'm here and ready. What would you like to work on?"
	default:
		return "Hi, I'm here. Tell me what you'd like to do."
	}
}
