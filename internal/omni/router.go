package omni

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var routerToolIDRe = regexp.MustCompile(`^[a-z0-9_]+$`)

type RouterResult struct {
	SelectedTools []string
	RawOutput     string
	Source        RouterSource
	LLMResponse   *OllamaChatResponse
}

type RouterSource string

const (
	RouterSourceOllama      RouterSource = "ollama"
	RouterSourceOllamaRetry RouterSource = "ollama_retry"
)

type RouterFailureKind string

const (
	RouterFailureClientUnavailable RouterFailureKind = "client_unavailable"
	RouterFailureRequest           RouterFailureKind = "request_failed"
	RouterFailureInvalidOutput     RouterFailureKind = "invalid_output"
)

type RouterError struct {
	Kind      RouterFailureKind
	Attempt   int
	RawOutput string
	Err       error
}

func (e RouterError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("router failed: kind=%s attempt=%d", e.Kind, e.Attempt)
	}
	return fmt.Sprintf("router failed: kind=%s attempt=%d: %v", e.Kind, e.Attempt, e.Err)
}

func (e RouterError) Unwrap() error {
	return e.Err
}

func ParseRouterCSV(raw string, registry Registry) ([]string, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return []string{}, nil
	}
	if strings.ContainsAny(normalized, " \t\n\r") {
		return nil, fmt.Errorf("router output contains whitespace")
	}

	parts := strings.Split(normalized, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}

	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("router output contains empty token")
		}
		if !routerToolIDRe.MatchString(part) {
			return nil, fmt.Errorf("invalid tool id %q", part)
		}

		tool, ok := registry.GetTool(part)
		if !ok {
			return nil, fmt.Errorf("unknown tool id %q", part)
		}

		if _, exists := seen[part]; exists {
			if tool.Repeatable {
				out = append(out, part)
				if len(out) > registry.MaxToolsPerStep {
					return nil, fmt.Errorf("tool count exceeds limit %d", registry.MaxToolsPerStep)
				}
			}
			continue
		}

		seen[part] = struct{}{}
		out = append(out, part)
		if len(out) > registry.MaxToolsPerStep {
			return nil, fmt.Errorf("tool count exceeds limit %d", registry.MaxToolsPerStep)
		}
	}

	return out, nil
}

func RouteTools(ctx context.Context, client *OllamaClient, registry Registry, userInput string) (RouterResult, error) {
	if client == nil {
		return RouterResult{}, RouterError{
			Kind:    RouterFailureClientUnavailable,
			Attempt: 0,
			Err:     fmt.Errorf("router LLM client is required"),
		}
	}

	first := callRouterLLM(ctx, client, registry, userInput, "")
	if first.err != nil {
		return RouterResult{Source: RouterSourceOllama}, RouterError{
			Kind:    RouterFailureRequest,
			Attempt: 1,
			Err:     first.err,
		}
	}

	parsed, parseErr := ParseRouterCSV(first.raw, registry)
	if parseErr == nil {
		return RouterResult{
			SelectedTools: parsed,
			RawOutput:     first.raw,
			Source:        RouterSourceOllama,
			LLMResponse:   first.resp,
		}, nil
	}

	second := callRouterLLM(ctx, client, registry, userInput, parseErr.Error())
	if second.err != nil {
		return RouterResult{
				RawOutput:   first.raw,
				Source:      RouterSourceOllamaRetry,
				LLMResponse: first.resp,
			}, RouterError{
				Kind:      RouterFailureRequest,
				Attempt:   2,
				RawOutput: first.raw,
				Err:       fmt.Errorf("router repair request failed after invalid first response: %w", second.err),
			}
	}
	parsed, parseErr = ParseRouterCSV(second.raw, registry)
	if parseErr != nil {
		return RouterResult{
				RawOutput:   second.raw,
				Source:      RouterSourceOllamaRetry,
				LLMResponse: second.resp,
			}, RouterError{
				Kind:      RouterFailureInvalidOutput,
				Attempt:   2,
				RawOutput: second.raw,
				Err:       parseErr,
			}
	}
	return RouterResult{
		SelectedTools: parsed,
		RawOutput:     second.raw,
		Source:        RouterSourceOllamaRetry,
		LLMResponse:   second.resp,
	}, nil
}

type routerCall struct {
	raw  string
	resp *OllamaChatResponse
	err  error
}

func callRouterLLM(ctx context.Context, client *OllamaClient, registry Registry, userInput, parseError string) routerCall {
	toolLines := registryToolSummary(registry)

	systemPrompt := "You are router_llm. Output only CSV of tool IDs. No spaces. No prose. No JSON. " +
		"If no tool is needed, return an empty string."

	userPrompt := strings.Builder{}
	userPrompt.WriteString("Available tools:\n")
	for _, line := range toolLines {
		userPrompt.WriteString(line)
		userPrompt.WriteString("\n")
	}
	userPrompt.WriteString("\nUser request:\n")
	userPrompt.WriteString(strings.TrimSpace(userInput))
	if strings.TrimSpace(parseError) != "" {
		userPrompt.WriteString("\n\nPrevious output was invalid: ")
		userPrompt.WriteString(parseError)
		userPrompt.WriteString(". Return corrected CSV now.")
	}

	resp, err := client.ChatRaw(ctx, OllamaChatRequest{
		Messages: []OllamaMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt.String()},
		},
		Options: map[string]interface{}{
			"temperature": 0,
		},
	})
	if err != nil {
		return routerCall{raw: "", resp: nil, err: err}
	}

	return routerCall{raw: strings.TrimSpace(resp.Content), resp: &resp, err: nil}
}

func registryToolSummary(registry Registry) []string {
	ids := registry.ToolIDs(true)
	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		tool, ok := registry.GetTool(id)
		if !ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", tool.ID, tool.Purpose))
	}
	return lines
}
