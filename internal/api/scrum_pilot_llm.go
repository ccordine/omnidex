package api

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const scrumPilotChatLLMTimeout = time.Hour

func scrumCardChatLLMContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, scrumPilotChatLLMTimeout)
}

func (s *Server) scrumPilotLLMChat(ctx context.Context, system, user string, meta llmContextTelemetryMeta) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("no llm client configured")
	}
	modelName, err := s.requiredDefaultLLMModel()
	if err != nil {
		return "", err
	}
	system = strings.TrimSpace(system)
	user = strings.TrimSpace(user)
	promptChars := llmPromptCharCount(system, user)

	if s.llmProviderName() == "ollama" {
		client := s.ollamaClientWithTimeout(scrumPilotChatLLMTimeout)
		generated, err := client.Chat(ctx, modelName, system, user)
		s.recordLLMContextUsage(ctx, llmContextSourceScrumPilot, modelName, "ollama", meta, promptChars, promptChars, false, 0, err)
		return generated, err
	}

	prompt := strings.TrimSpace(system + "\n\n" + user)
	generated, err := s.llmClient.Generate(ctx, modelName, prompt)
	s.recordLLMContextUsage(ctx, llmContextSourceScrumPilot, modelName, s.llmProviderName(), meta, promptChars, len(prompt), false, 0, err)
	return generated, err
}
