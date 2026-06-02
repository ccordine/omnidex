package scrumcardllm

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/llm"
)

// ChatClient supports direct chat completions without ephemeral context modelfiles.
type ChatClient interface {
	Chat(ctx context.Context, model, system, user string) (string, error)
}

// AsChatClient returns a ChatClient when the underlying LLM supports /api/chat.
func AsChatClient(client LLMClient) ChatClient {
	if client == nil {
		return nil
	}
	if chat, ok := client.(ChatClient); ok {
		return chat
	}
	if routed, ok := client.(*llm.RoutedClient); ok && routed.Generation != nil {
		if chat, ok := routed.Generation.(ChatClient); ok {
			return chat
		}
	}
	return nil
}

func chatOrGenerate(ctx context.Context, client LLMClient, modelName, system, user string) (string, error) {
	if client == nil {
		return "", fmt.Errorf("no llm client configured")
	}
	system = strings.TrimSpace(system)
	user = strings.TrimSpace(user)
	if chat := AsChatClient(client); chat != nil {
		return chat.Chat(ctx, modelName, system, user)
	}
	prompt := strings.TrimSpace(system + "\n\n" + user)
	return client.Generate(ctx, modelName, prompt)
}
