package assemblyline

import (
	"fmt"
	"strings"
)

// DecodeTypeScriptFunctionModelResponse owns only the registered raw-code
// response framing. It returns the normalized candidate even when parser or
// policy validation rejects it so code can retain and localize that exact
// candidate without replaying provider framing.
func DecodeTypeScriptFunctionModelResponse(
	contract TypeScriptFunctionContract,
	raw string,
) (string, error) {
	content := strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if content == "" {
		return "", fmt.Errorf("TypeScript model response is empty")
	}
	if strings.HasPrefix(content, "```") {
		newline := strings.IndexByte(content, '\n')
		if newline < 0 {
			return "", fmt.Errorf("TypeScript model response has an unterminated code fence")
		}
		language := strings.TrimSpace(strings.TrimPrefix(content[:newline], "```"))
		if !allowedTypeScriptFence(language, contract.TSX) {
			return "", fmt.Errorf("TypeScript model response uses unsupported code fence %q", language)
		}
		if !strings.HasSuffix(content, "\n```") {
			return "", fmt.Errorf("TypeScript model response code fence is not the complete response")
		}
		content = strings.TrimSpace(content[newline+1 : len(content)-len("\n```")])
		if content == "" {
			return "", fmt.Errorf("TypeScript model response code fence is empty")
		}
	} else if strings.Contains(content, "```") {
		return "", fmt.Errorf("TypeScript model response contains narrative or unmatched code fencing")
	}

	if control := trailingTypeScriptProviderControl(content); control != "" {
		return "", fmt.Errorf("TypeScript model response contains provider control framing %q", control)
	}
	_, err := ParseTypeScriptFunction(contract, content)
	return content, err
}

func allowedTypeScriptFence(language string, tsx bool) bool {
	switch strings.ToLower(language) {
	case "typescript", "ts":
		return true
	case "tsx":
		return tsx
	default:
		return false
	}
}

func trailingTypeScriptProviderControl(content string) string {
	lastBrace := strings.LastIndex(content, "}")
	if lastBrace < 0 || lastBrace == len(content)-1 {
		return ""
	}
	tail := content[lastBrace+1:]
	for _, marker := range []string{"<|endoftext|>", "<|im_start|>", "<|im_end|>"} {
		if strings.Contains(tail, marker) {
			return marker
		}
	}
	return ""
}
