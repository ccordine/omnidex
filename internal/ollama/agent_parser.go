package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/llm"
)

const maxParserResponseBytes = 2 * 1024 * 1024

// Parser supports only semantic response extraction. It has no tool, workspace,
// native-session, or execution capability.
type Parser struct {
	client        *Client
	contextTokens int
}

func NewParser(endpoint string, timeout time.Duration, contextTokens int) (*Parser, error) {
	if strings.TrimSpace(endpoint) == "" {
		return nil, fmt.Errorf("local response parser requires an Ollama endpoint")
	}
	if err := llm.ValidateInferenceContextTokens(contextTokens); err != nil {
		return nil, err
	}
	return &Parser{client: New(endpoint, "", "", timeout), contextTokens: contextTokens}, nil
}

type parserRequest struct {
	Model    string         `json:"model"`
	Prompt   string         `json:"prompt"`
	Format   map[string]any `json:"format"`
	Stream   bool           `json:"stream"`
	Raw      bool           `json:"raw"`
	Shift    bool           `json:"shift"`
	Truncate bool           `json:"truncate"`
	Options  parserOptions  `json:"options"`
}

type parserOptions struct {
	ContextTokens int     `json:"num_ctx"`
	OutputTokens  int     `json:"num_predict"`
	Temperature   float64 `json:"temperature"`
	Seed          int     `json:"seed"`
}

type parserResponse struct {
	Model           string `json:"model"`
	Response        string `json:"response"`
	Done            bool   `json:"done"`
	DoneReason      string `json:"done_reason"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
}

func (parser *Parser) Run(ctx context.Context, call codingagent.Assignment, event func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	result := codingagent.Result{Provider: "ollama", Model: call.Route.Model}
	if event == nil {
		return result, fmt.Errorf("local response parser requires an evidence sink")
	}
	if call.Role.IsNative() || call.Route.Provider != "ollama" || call.Session != nil ||
		call.Route.Effort != "" || call.Route.ServiceTier != "" {
		return result, fmt.Errorf("Ollama response parsing requires a tool-free parser role without native session or inference options")
	}
	if err := call.ValidateTaskContext(); err != nil {
		return result, err
	}
	instruction, err := codingagent.RenderParserPrompt(call)
	if err != nil {
		return result, err
	}
	contextText, err := codingagent.RenderParserContext(call)
	if err != nil {
		return result, err
	}
	request := parserRequest{Model: call.Route.Model, Prompt: contextText + "\n\n" + instruction,
		Format: codingagent.ParserOutputSchema(call), Options: parserOptions{
			ContextTokens: parser.contextTokens, OutputTokens: parser.contextTokens / 2,
		}}
	if len(request.Prompt) > parser.contextTokens*8 {
		return result, fmt.Errorf("response parser input exceeds its local context byte boundary")
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return result, err
	}
	if err := parserEvent(event, "request", payload); err != nil {
		return result, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, parser.client.baseURL+"/api/generate", bytes.NewReader(payload))
	if err != nil {
		return result, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, _, err := parser.client.doExactProviderRequest(httpRequest)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxParserResponseBytes+1))
	if err != nil {
		return result, err
	}
	if len(body) > maxParserResponseBytes {
		return result, fmt.Errorf("local parser response exceeds %d bytes", maxParserResponseBytes)
	}
	if !json.Valid(body) {
		return result, fmt.Errorf("local parser returned invalid provider JSON (HTTP %d)", response.StatusCode)
	}
	if err := parserEvent(event, "response", body); err != nil {
		return result, err
	}
	if response.StatusCode != http.StatusOK {
		return result, fmt.Errorf("local parser failed: HTTP %d: %s", response.StatusCode, body)
	}
	var parsed parserResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return result, err
	}
	if !MatchesOllamaModel(call.Route.Model, parsed.Model) || !parsed.Done || parsed.DoneReason != "stop" ||
		parsed.PromptEvalCount < 1 || parsed.EvalCount < 1 || parsed.PromptEvalCount+parsed.EvalCount > parser.contextTokens {
		return result, fmt.Errorf("local parser returned an incomplete, truncated, oversized or different-model result")
	}
	if !json.Valid([]byte(parsed.Response)) {
		return result, fmt.Errorf("local parser output does not contain one JSON result")
	}
	result.Content, err = codingagent.MapParserResponse(call, json.RawMessage(parsed.Response))
	result.Completed = err == nil
	return result, err
}

func parserEvent(event func(codingagent.NativeEvent) error, phase string, payload json.RawMessage) error {
	frame, err := json.Marshal(struct {
		Provider string          `json:"provider"`
		Phase    string          `json:"phase"`
		Payload  json.RawMessage `json:"payload"`
	}{Provider: "ollama", Phase: phase, Payload: payload})
	if err != nil {
		return err
	}
	return event(codingagent.NativeEvent{Frame: frame})
}
