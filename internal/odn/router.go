package odn

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
	Source        string
	ParseError    string
	LLMResponse   *OllamaChatResponse
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

func RouteTools(ctx context.Context, client *OllamaClient, registry Registry, userInput string) RouterResult {
	if client == nil {
		selected := heuristicRoute(userInput)
		return RouterResult{
			SelectedTools: selected,
			RawOutput:     strings.Join(selected, ","),
			Source:        "heuristic",
		}
	}

	first := callRouterLLM(ctx, client, registry, userInput, "")
	if first.err == nil {
		parsed, parseErr := ParseRouterCSV(first.raw, registry)
		if parseErr == nil {
			return RouterResult{SelectedTools: parsed, RawOutput: first.raw, Source: "ollama", LLMResponse: first.resp}
		}

		second := callRouterLLM(ctx, client, registry, userInput, parseErr.Error())
		if second.err == nil {
			parsed2, parseErr2 := ParseRouterCSV(second.raw, registry)
			if parseErr2 == nil {
				return RouterResult{SelectedTools: parsed2, RawOutput: second.raw, Source: "ollama_retry", LLMResponse: second.resp}
			}
			fallback := heuristicRoute(userInput)
			return RouterResult{
				SelectedTools: fallback,
				RawOutput:     second.raw,
				Source:        "heuristic_after_parse_fail",
				ParseError:    parseErr2.Error(),
				LLMResponse:   second.resp,
			}
		}

		fallback := heuristicRoute(userInput)
		return RouterResult{
			SelectedTools: fallback,
			RawOutput:     first.raw,
			Source:        "heuristic_after_retry_error",
			ParseError:    parseErr.Error(),
			LLMResponse:   first.resp,
		}
	}

	fallback := heuristicRoute(userInput)
	return RouterResult{
		SelectedTools: fallback,
		RawOutput:     strings.Join(fallback, ","),
		Source:        "heuristic_after_ollama_error",
		ParseError:    first.err.Error(),
		LLMResponse:   first.resp,
	}
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

func heuristicRoute(userInput string) []string {
	normalized := strings.ToLower(strings.TrimSpace(userInput))
	if normalized == "" {
		return []string{}
	}

	result := make([]string, 0, 3)
	add := func(id string) {
		for _, current := range result {
			if current == id {
				return
			}
		}
		result = append(result, id)
	}

	isScaffold := (strings.Contains(normalized, "make") || strings.Contains(normalized, "create") || strings.Contains(normalized, "build") || strings.Contains(normalized, "scaffold")) &&
		strings.Contains(normalized, "project") &&
		(strings.Contains(normalized, "go") || strings.Contains(normalized, "golang")) &&
		strings.Contains(normalized, "html")
	if isScaffold {
		add("scaffold_go_html_project")
	}

	hasCommandLanguage := strings.Contains(normalized, "command") ||
		strings.Contains(normalized, "terminal") ||
		strings.Contains(normalized, "shell") ||
		strings.Contains(normalized, "run ") ||
		strings.Contains(normalized, "install ")
	if hasCommandLanguage {
		add("linux_command")
	}

	if len(result) > 0 {
		add("verification_gate")
	}

	return result
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
