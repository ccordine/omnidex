package worker

import (
	"fmt"
	"strings"
)

func normalizeDirectCodingTypeScriptResponse(raw string) (string, error) {
	content := strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if content == "" {
		return "", fmt.Errorf("TypeScript fragment response is empty")
	}
	if !strings.HasPrefix(content, "```") {
		if strings.Contains(content, "```") {
			return "", fmt.Errorf("TypeScript fragment has a malformed code envelope")
		}
		return content, nil
	}
	lines := strings.Split(content, "\n")
	if len(lines) < 3 {
		return "", fmt.Errorf("TypeScript fragment code envelope is incomplete")
	}
	opening := strings.ToLower(strings.TrimSpace(lines[0]))
	if opening != "```typescript" && opening != "```ts" && opening != "```tsx" {
		return "", fmt.Errorf("TypeScript fragment code envelope language %q is unsupported", strings.TrimPrefix(opening, "```"))
	}
	if strings.TrimSpace(lines[len(lines)-1]) != "```" {
		return "", fmt.Errorf("TypeScript fragment code envelope has trailing content or no closing fence")
	}
	body := strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
	if body == "" || strings.Contains(body, "```") {
		return "", fmt.Errorf("TypeScript fragment code envelope must contain exactly one non-empty code body")
	}
	return body, nil
}
