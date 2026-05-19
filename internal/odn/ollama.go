package odn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OllamaClient struct {
	Endpoint string
	Model    string
	Client   *http.Client
}

type OllamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OllamaChatRequest struct {
	Messages  []OllamaMessage
	Format    interface{}
	Options   map[string]interface{}
	KeepAlive string
}

type OllamaChatResponse struct {
	Model              string
	CreatedAt          string
	Content            string
	Thinking           string
	Done               bool
	DoneReason         string
	TotalDuration      int64
	LoadDuration       int64
	PromptEvalCount    int64
	PromptEvalDuration int64
	EvalCount          int64
	EvalDuration       int64
	RequestJSON        string
	ResponseJSON       string
}

func NewOllamaClient(endpoint, model string) *OllamaClient {
	ep := strings.TrimSpace(endpoint)
	if ep == "" {
		ep = defaultOllamaEndpoint
	}
	m := strings.TrimSpace(model)
	if m == "" {
		m = defaultOllamaModel
	}
	return &OllamaClient{
		Endpoint: ep,
		Model:    m,
		Client: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

func (c *OllamaClient) ChatRaw(ctx context.Context, req OllamaChatRequest) (OllamaChatResponse, error) {
	if len(req.Messages) == 0 {
		return OllamaChatResponse{}, fmt.Errorf("ollama request requires at least one message")
	}

	payload := map[string]interface{}{
		"model":    c.Model,
		"stream":   false,
		"messages": req.Messages,
	}
	if req.Format != nil {
		payload["format"] = req.Format
	}
	if len(req.Options) > 0 {
		payload["options"] = req.Options
	}
	if strings.TrimSpace(req.KeepAlive) != "" {
		payload["keep_alive"] = req.KeepAlive
	}

	blob, err := json.Marshal(payload)
	if err != nil {
		return OllamaChatResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(blob))
	if err != nil {
		return OllamaChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return OllamaChatResponse{}, err
	}
	defer resp.Body.Close()

	respBlob, err := io.ReadAll(resp.Body)
	if err != nil {
		return OllamaChatResponse{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return OllamaChatResponse{}, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, truncateForError(string(respBlob)))
	}

	var decoded struct {
		Model      string `json:"model"`
		CreatedAt  string `json:"created_at"`
		Done       bool   `json:"done"`
		DoneReason string `json:"done_reason"`
		Message    struct {
			Role     string `json:"role"`
			Content  string `json:"content"`
			Thinking string `json:"thinking"`
		} `json:"message"`
		Error              string `json:"error"`
		TotalDuration      int64  `json:"total_duration"`
		LoadDuration       int64  `json:"load_duration"`
		PromptEvalCount    int64  `json:"prompt_eval_count"`
		PromptEvalDuration int64  `json:"prompt_eval_duration"`
		EvalCount          int64  `json:"eval_count"`
		EvalDuration       int64  `json:"eval_duration"`
	}

	if err := json.Unmarshal(respBlob, &decoded); err != nil {
		return OllamaChatResponse{}, err
	}
	if strings.TrimSpace(decoded.Error) != "" {
		return OllamaChatResponse{}, fmt.Errorf("%s", decoded.Error)
	}

	content := strings.TrimSpace(decoded.Message.Content)
	if content == "" {
		return OllamaChatResponse{}, fmt.Errorf("ollama returned empty content")
	}

	return OllamaChatResponse{
		Model:              decoded.Model,
		CreatedAt:          decoded.CreatedAt,
		Content:            content,
		Thinking:           strings.TrimSpace(decoded.Message.Thinking),
		Done:               decoded.Done,
		DoneReason:         decoded.DoneReason,
		TotalDuration:      decoded.TotalDuration,
		LoadDuration:       decoded.LoadDuration,
		PromptEvalCount:    decoded.PromptEvalCount,
		PromptEvalDuration: decoded.PromptEvalDuration,
		EvalCount:          decoded.EvalCount,
		EvalDuration:       decoded.EvalDuration,
		RequestJSON:        string(blob),
		ResponseJSON:       string(respBlob),
	}, nil
}

func (c *OllamaClient) Chat(ctx context.Context, workspacePath string, history []Message, userInput string) (string, error) {
	messages := make([]OllamaMessage, 0, maxConversationHistoryMessages+2)
	messages = append(messages, OllamaMessage{
		Role:    "system",
		Content: MinimalOutputContract + " Practical. Current workspace: " + workspacePath + ".",
	})

	start := 0
	if len(history) > maxConversationHistoryMessages {
		start = len(history) - maxConversationHistoryMessages
	}
	for _, msg := range history[start:] {
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}
		messages = append(messages, OllamaMessage{Role: msg.Role, Content: msg.Content})
	}
	messages = append(messages, OllamaMessage{Role: "user", Content: userInput})

	result, err := c.ChatRaw(ctx, OllamaChatRequest{
		Messages: messages,
		Options: map[string]interface{}{
			"temperature": 0.2,
		},
	})
	if err != nil {
		return "", err
	}
	return result.Content, nil
}

func truncateForError(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) <= 220 {
		return trimmed
	}
	return trimmed[:220] + "..."
}
