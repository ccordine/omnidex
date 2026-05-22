package omni

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type OllamaClient struct {
	Endpoint         string
	Model            string
	DefaultKeepAlive string
	DefaultNumCtx    int
	Client           *http.Client
}

type OllamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OllamaChatRequest struct {
	Messages      []OllamaMessage
	Format        interface{}
	Options       map[string]interface{}
	KeepAlive     string
	ContextSystem string
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

type OllamaPrewarmResult struct {
	Model              string `json:"model"`
	Endpoint           string `json:"endpoint"`
	KeepAlive          string `json:"keep_alive,omitempty"`
	NumCtx             int    `json:"num_ctx,omitempty"`
	Done               bool   `json:"done"`
	TotalDuration      int64  `json:"total_duration,omitempty"`
	LoadDuration       int64  `json:"load_duration,omitempty"`
	PromptEvalCount    int64  `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64  `json:"prompt_eval_duration,omitempty"`
	EvalCount          int64  `json:"eval_count,omitempty"`
	EvalDuration       int64  `json:"eval_duration,omitempty"`
	Diagnosis          string `json:"diagnosis,omitempty"`
}

var omniContextModelCounter uint64

const defaultOllamaRequestTimeout = 10 * time.Minute
const defaultOllamaContextCharsPerToken = 24
const minOllamaContextBudgetChars = 24000
const defaultOllamaContextBudgetChars = 100000

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
			Timeout: defaultOllamaRequestTimeout,
		},
	}
}

func (c *OllamaClient) ConfigureRuntime(defaultKeepAlive string, defaultNumCtx int) {
	c.DefaultKeepAlive = strings.TrimSpace(defaultKeepAlive)
	if defaultNumCtx > 0 {
		c.DefaultNumCtx = defaultNumCtx
	}
}

func (c *OllamaClient) Prewarm(ctx context.Context) (OllamaPrewarmResult, error) {
	resp, err := c.ChatRaw(ctx, OllamaChatRequest{
		Messages: []OllamaMessage{
			{Role: "system", Content: "Return exactly: ok"},
			{Role: "user", Content: "ok"},
		},
		Options: map[string]interface{}{
			"temperature": 0,
			"num_predict": 1,
		},
	})
	result := OllamaPrewarmResult{
		Model:     c.Model,
		Endpoint:  c.Endpoint,
		KeepAlive: c.DefaultKeepAlive,
		NumCtx:    c.DefaultNumCtx,
	}
	if err != nil {
		result.Diagnosis = classifyStructuredLLMFailure(err)
		return result, err
	}
	result.Done = resp.Done
	result.TotalDuration = resp.TotalDuration
	result.LoadDuration = resp.LoadDuration
	result.PromptEvalCount = resp.PromptEvalCount
	result.PromptEvalDuration = resp.PromptEvalDuration
	result.EvalCount = resp.EvalCount
	result.EvalDuration = resp.EvalDuration
	return result, nil
}

func (c *OllamaClient) ChatRaw(ctx context.Context, req OllamaChatRequest) (OllamaChatResponse, error) {
	if len(req.Messages) == 0 {
		return OllamaChatResponse{}, fmt.Errorf("ollama request requires at least one message")
	}
	req = budgetOllamaChatRequest(req, c.effectivePromptBudgetChars(req))
	model := c.Model
	messages := req.Messages
	if strings.TrimSpace(req.ContextSystem) != "" {
		contextModel, err := c.createChatContextModel(ctx, req.ContextSystem)
		if err != nil {
			return OllamaChatResponse{}, err
		}
		defer c.bestEffortDeleteChatContextModel(contextModel)
		model = contextModel
		messages = nonSystemMessages(messages)
		if len(messages) == 0 {
			return OllamaChatResponse{}, fmt.Errorf("ollama context request requires at least one non-system message")
		}
	}

	payload := map[string]interface{}{
		"model":    model,
		"stream":   false,
		"messages": messages,
	}
	if req.Format != nil {
		payload["format"] = req.Format
	}
	options := map[string]interface{}{}
	for key, value := range req.Options {
		options[key] = value
	}
	if c.DefaultNumCtx > 0 {
		if _, exists := options["num_ctx"]; !exists {
			options["num_ctx"] = c.DefaultNumCtx
		}
	}
	if len(options) > 0 {
		payload["options"] = options
	}
	keepAlive := strings.TrimSpace(req.KeepAlive)
	if keepAlive == "" {
		keepAlive = strings.TrimSpace(c.DefaultKeepAlive)
	}
	if keepAlive != "" {
		payload["keep_alive"] = keepAlive
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

func nonSystemMessages(messages []OllamaMessage) []OllamaMessage {
	out := make([]OllamaMessage, 0, len(messages))
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "system") {
			continue
		}
		out = append(out, message)
	}
	return out
}

func (c *OllamaClient) effectivePromptBudgetChars(req OllamaChatRequest) int {
	numCtx := c.DefaultNumCtx
	if value, ok := req.Options["num_ctx"]; ok {
		switch typed := value.(type) {
		case int:
			numCtx = typed
		case int64:
			numCtx = int(typed)
		case float64:
			numCtx = int(typed)
		case json.Number:
			if parsed, err := typed.Int64(); err == nil {
				numCtx = int(parsed)
			}
		}
	}
	if numCtx <= 0 {
		return defaultOllamaContextBudgetChars
	}
	return maxInt(minOllamaContextBudgetChars, numCtx*defaultOllamaContextCharsPerToken)
}

func budgetOllamaChatRequest(req OllamaChatRequest, budget int) OllamaChatRequest {
	if budget <= 0 {
		budget = defaultOllamaContextBudgetChars
	}
	for attempt := 0; attempt < 8 && approxOllamaRequestChars(req) > budget; attempt++ {
		index := largestBudgetableOllamaMessage(req.Messages)
		if index < 0 {
			if len(req.ContextSystem) > budget/2 {
				req.ContextSystem = truncateOllamaBudgetString(req.ContextSystem, budget/2)
				continue
			}
			break
		}
		current := req.Messages[index].Content
		target := maxInt(1200, len(current)/2)
		req.Messages[index].Content = compactOllamaMessageContent(current, target)
		if req.Messages[index].Content == current {
			req.Messages[index].Content = truncateOllamaBudgetString(current, target)
		}
	}
	return req
}

func largestBudgetableOllamaMessage(messages []OllamaMessage) int {
	index := -1
	size := 0
	for i, message := range messages {
		if strings.TrimSpace(message.Content) == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(message.Role), "system") && len(message.Content) < 4000 {
			continue
		}
		if len(message.Content) > size {
			index = i
			size = len(message.Content)
		}
	}
	return index
}

func compactOllamaMessageContent(content string, target int) string {
	trimmed := strings.TrimSpace(content)
	if len(trimmed) <= target {
		return trimmed
	}
	var decoded interface{}
	if json.Unmarshal([]byte(trimmed), &decoded) == nil {
		compacted := compactJSONValueForOllamaBudget(decoded, target, 0)
		if blob, err := json.Marshal(compacted); err == nil && len(blob) < len(trimmed) {
			if len(blob) <= target {
				return string(blob)
			}
			return truncateOllamaBudgetString(string(blob), target)
		}
	}
	return truncateOllamaBudgetString(trimmed, target)
}

func compactJSONValueForOllamaBudget(value interface{}, target int, depth int) interface{} {
	if depth > 8 {
		return "[context truncated]"
	}
	switch typed := value.(type) {
	case string:
		return truncateOllamaBudgetString(typed, maxInt(240, target/6))
	case []interface{}:
		if len(typed) == 0 {
			return typed
		}
		limit := maxInt(1, minInt(len(typed), maxInt(2, target/1800)))
		out := make([]interface{}, 0, limit+1)
		start := 0
		if len(typed) > limit {
			start = len(typed) - limit
			out = append(out, fmt.Sprintf("[context compacted: omitted %d earlier items]", start))
		}
		for _, item := range typed[start:] {
			out = append(out, compactJSONValueForOllamaBudget(item, target/maxInt(1, limit), depth+1))
		}
		return out
	case map[string]interface{}:
		out := map[string]interface{}{}
		for key, item := range typed {
			lower := strings.ToLower(key)
			nextTarget := target / 4
			if lower == "current_prompt" || lower == "prompt" || lower == "objective_ledger" || lower == "completed_actions" || lower == "loop_state" || lower == "pending_objective_ids" {
				nextTarget = target
			}
			out[key] = compactJSONValueForOllamaBudget(item, maxInt(400, nextTarget), depth+1)
		}
		return out
	default:
		return value
	}
}

func truncateOllamaBudgetString(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || value == "" {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	marker := "\n[context truncated by request budget guard]"
	if limit <= len(marker)+32 {
		return value[:limit]
	}
	return value[:limit-len(marker)] + marker
}

func (c *OllamaClient) createChatContextModel(ctx context.Context, systemPrompt string) (string, error) {
	modelName := buildOmniContextModelName(c.Model, systemPrompt)
	modelfile := buildOmniContextModelfile(c.Model, systemPrompt)
	if _, err := persistOmniContextModelfile(modelName, modelfile); err != nil {
		return "", err
	}

	payload, err := json.Marshal(map[string]interface{}{
		"name":      modelName,
		"model":     modelName,
		"from":      c.Model,
		"modelfile": modelfile,
		"stream":    false,
	})
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.ollamaBaseURL()+"/api/create", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBlob, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama create context model returned status %d: %s", resp.StatusCode, truncateForError(string(respBlob)))
	}
	return modelName, nil
}

func (c *OllamaClient) bestEffortDeleteChatContextModel(modelName string) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	payload, err := json.Marshal(map[string]interface{}{
		"name":  modelName,
		"model": modelName,
	})
	if err != nil {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.ollamaBaseURL()+"/api/delete", bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Client.Do(req)
	if err != nil {
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func (c *OllamaClient) ollamaBaseURL() string {
	endpoint := strings.TrimRight(strings.TrimSpace(c.Endpoint), "/")
	for _, suffix := range []string{"/api/chat", "/api/generate"} {
		if strings.HasSuffix(endpoint, suffix) {
			return strings.TrimSuffix(endpoint, suffix)
		}
	}
	return endpoint
}

func buildOmniContextModelName(baseModel string, systemPrompt string) string {
	base := sanitizeOmniModelNameComponent(baseModel)
	if base == "" {
		base = "model"
	}
	if len(base) > 24 {
		base = base[:24]
	}
	sum := sha1.Sum([]byte(systemPrompt))
	seq := atomic.AddUint64(&omniContextModelCounter, 1)
	return strings.ToLower(strings.Join([]string{
		"omnictx",
		base,
		fmt.Sprintf("%x", sum[:4]),
		strconv.FormatUint(seq, 10),
	}, "-"))
}

func sanitizeOmniModelNameComponent(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			b.WriteRune(ch)
			lastDash = false
			continue
		}
		if lastDash {
			continue
		}
		b.WriteByte('-')
		lastDash = true
	}
	return strings.Trim(b.String(), "-")
}

func buildOmniContextModelfile(baseModel string, systemPrompt string) string {
	baseModel = strings.TrimSpace(baseModel)
	if baseModel == "" {
		baseModel = defaultOllamaModel
	}
	systemPrompt = strings.ReplaceAll(strings.TrimSpace(systemPrompt), `"""`, `\"\"\"`)
	return strings.Join([]string{
		"FROM " + baseModel,
		"SYSTEM \"\"\"",
		systemPrompt,
		"\"\"\"",
	}, "\n")
}

func persistOmniContextModelfile(modelName, modelfile string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("resolve home directory for omni modelfile storage: %w", err)
	}
	dir := filepath.Join(home, ".omni", "modelfiles")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create omni modelfile directory %s: %w", dir, err)
	}
	path := filepath.Join(dir, sanitizeOmniModelNameComponent(modelName)+".Modelfile")
	if err := os.WriteFile(path, []byte(modelfile), 0o600); err != nil {
		return "", fmt.Errorf("write omni modelfile %s: %w", path, err)
	}
	return path, nil
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
