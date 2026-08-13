package main

import (
	"strings"
)

func summarizeLLMTraceContext(key, value string, maxChars int) (string, string) {
	scope := traceKVField(value, "scope")
	model := traceKVField(value, "model")
	chars := traceKVField(value, "prompt_chars")
	if strings.EqualFold(strings.TrimSpace(key), "llm_response") {
		chars = traceKVField(value, "response_chars")
	}
	if chars == "" && strings.EqualFold(strings.TrimSpace(key), "llm_error") {
		chars = traceKVField(value, "error")
	}

	kind := "Trace"
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "llm_prompt":
		kind = "Prompt"
	case "llm_response":
		kind = "Response"
	case "llm_error":
		kind = "Warn"
	}

	parts := make([]string, 0, 4)
	if scope != "" {
		parts = append(parts, "scope="+scope)
	}
	if model != "" {
		parts = append(parts, "model="+model)
	}
	if chars != "" {
		if strings.EqualFold(strings.TrimSpace(key), "llm_error") {
			parts = append(parts, "error="+chars)
		} else {
			parts = append(parts, "chars="+chars)
		}
	}
	if len(parts) == 0 {
		return kind, compactProgressValue(value, maxChars)
	}
	return kind, compactProgressValue(strings.Join(parts, " "), maxChars)
}

func summarizePreparedModelContext(value string, maxChars int) (string, string) {
	scope := traceKVField(value, "scope")
	baseModel := traceKVField(value, "base_model")
	contextModel := traceKVField(value, "context_model")
	modelfilePath := traceKVField(value, "modelfile_path")
	promptHint := traceKVField(value, "prompt_hint")

	parts := make([]string, 0, 6)
	if scope != "" {
		parts = append(parts, "scope="+scope)
	}
	if baseModel != "" {
		parts = append(parts, "base_model="+baseModel)
	}
	if contextModel != "" {
		parts = append(parts, "context_model="+contextModel)
	}
	if modelfilePath != "" {
		parts = append(parts, "modelfile="+modelfilePath)
	}
	if promptHint != "" {
		parts = append(parts, "prompt_hint="+promptHint)
	}
	if len(parts) == 0 {
		return "Model", compactProgressValue(value, maxChars)
	}
	return "Model", compactProgressValue(strings.Join(parts, " "), maxChars)
}

func llmTraceBody(value string) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if len(lines) >= 4 {
		first := strings.ToLower(strings.TrimSpace(lines[0]))
		second := strings.ToLower(strings.TrimSpace(lines[1]))
		third := strings.ToLower(strings.TrimSpace(lines[2]))
		if strings.HasPrefix(first, "scope=") &&
			strings.HasPrefix(second, "model=") &&
			(strings.HasPrefix(third, "response_chars=") || strings.HasPrefix(third, "prompt_chars=") || strings.HasPrefix(third, "error=")) {
			return strings.TrimSpace(strings.Join(lines[3:], "\n"))
		}
	}
	return strings.TrimSpace(value)
}

func traceKVField(value, key string) string {
	needle := strings.ToLower(strings.TrimSpace(key)) + "="
	for _, line := range strings.Split(strings.TrimSpace(value), "\n") {
		clean := strings.TrimSpace(line)
		lower := strings.ToLower(clean)
		if !strings.HasPrefix(lower, needle) {
			continue
		}
		return strings.TrimSpace(clean[len(needle):])
	}
	return ""
}

func truncateForWatch(value string, maxChars int) string {
	text := strings.TrimSpace(value)
	if maxChars <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return string(runes[:maxChars]) + "\n...[truncated]"
}

func compactProgressValue(value string, maxChars int) string {
	text := strings.TrimSpace(strings.ReplaceAll(value, "\n", " | "))
	if maxChars <= 0 || maxChars > 320 {
		maxChars = 320
	}
	runes := []rune(text)
	if len(runes) <= maxChars {
		return text
	}
	return string(runes[:maxChars]) + "...[truncated]"
}

func indentBlock(value, prefix string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
