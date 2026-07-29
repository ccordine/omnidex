package omni

type OllamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OllamaChatRequest is the minimal provider-neutral request shape retained for
// datasource analysis. Model execution itself is owned by internal/llm.
type OllamaChatRequest struct {
	Messages      []OllamaMessage `json:"messages"`
	Format        any             `json:"format,omitempty"`
	Options       map[string]any  `json:"options,omitempty"`
	KeepAlive     string          `json:"keep_alive,omitempty"`
	ContextSystem string          `json:"context_system,omitempty"`
}

type OllamaChatResponse struct {
	Model      string `json:"model,omitempty"`
	Content    string `json:"content"`
	Thinking   string `json:"thinking,omitempty"`
	Done       bool   `json:"done"`
	DoneReason string `json:"done_reason,omitempty"`
}
