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

func (s *Server) scrumPilotLLMChat(ctx context.Context, system, user string) (string, error) {
	if s.llmClient == nil {
		return "", fmt.Errorf("no llm client configured")
	}
	modelName := firstNonEmpty(s.ollamaDefaultModel, "llama3.2")
	system = strings.TrimSpace(system)
	user = strings.TrimSpace(user)

	client := s.ollamaClientWithTimeout(scrumPilotChatLLMTimeout)
	if client != nil {
		return client.Chat(ctx, modelName, system, user)
	}

	// Non-Ollama providers: single generate call, no context-modelfile path when possible.
	if routed, ok := s.llmClient.(*llm.RoutedClient); ok && routed.Generation != nil {
		if direct, ok := routed.Generation.(*ollama.Client); ok {
			return direct.Chat(ctx, modelName, system, user)
		}
	}
	return s.llmClient.Generate(ctx, modelName, strings.TrimSpace(system+"\n\n"+user))
}
