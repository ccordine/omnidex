package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
)

const scrumPilotChatLLMTimeout = time.Hour

func scrumCardChatLLMContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), scrumPilotChatLLMTimeout)
}

func (s *Server) scrumPilotLLMChat(ctx context.Context, system, user string, meta llmContextTelemetryMeta) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("no llm client configured")
	}
	modelName := firstNonEmpty(s.ollamaDefaultModel, "llama3.2")
	system = strings.TrimSpace(system)
	user = strings.TrimSpace(user)
	promptChars := llmPromptCharCount(system, user)

	client := s.ollamaClientWithTimeout(scrumPilotChatLLMTimeout)
	if client != nil {
		generated, err := client.Chat(ctx, modelName, system, user)
		s.recordLLMContextUsage(ctx, llmContextSourceScrumPilot, modelName, "ollama", meta, promptChars, promptChars, false, 0, err)
		return generated, err
	}

	// Non-Ollama providers: single generate call, no context-modelfile path when possible.
	if routed, ok := s.llmClient.(*llm.RoutedClient); ok && routed.Generation != nil {
		if direct, ok := routed.Generation.(*ollama.Client); ok {
			generated, err := direct.Chat(ctx, modelName, system, user)
			s.recordLLMContextUsage(ctx, llmContextSourceScrumPilot, modelName, "ollama", meta, promptChars, promptChars, false, 0, err)
			return generated, err
		}
	}
	prompt := strings.TrimSpace(system + "\n\n" + user)
	generated, err := s.llmClient.Generate(ctx, modelName, prompt)
	s.recordLLMContextUsage(ctx, llmContextSourceScrumPilot, modelName, s.llmProviderName(), meta, promptChars, len(prompt), false, 0, err)
	return generated, err
}
