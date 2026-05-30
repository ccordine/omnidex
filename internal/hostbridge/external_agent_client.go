package hostbridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ExternalAgentRunRequest struct {
	Agent     string
	APIKey    string
	Model     string
	Workspace string
	Prompt    string
	CodexPath string
}

func (c *Client) RunExternalAgent(ctx context.Context, req ExternalAgentRunRequest) (io.ReadCloser, error) {
	if c == nil {
		return nil, fmt.Errorf("host bridge client is nil")
	}
	agent := strings.ToLower(strings.TrimSpace(req.Agent))
	path := "/v1/cursor/run"
	switch agent {
	case "cursor":
		path = "/v1/cursor/run"
	case "codex":
		path = "/v1/codex/run"
	default:
		return nil, fmt.Errorf("unsupported external agent %q", req.Agent)
	}

	body := map[string]string{
		"api_key":   strings.TrimSpace(req.APIKey),
		"model":     strings.TrimSpace(req.Model),
		"workspace": strings.TrimSpace(req.Workspace),
		"prompt":    strings.TrimSpace(req.Prompt),
	}
	if agent == "codex" {
		body["codex_path"] = strings.TrimSpace(req.CodexPath)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.applyAuth(httpReq)

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := readResponseBody(resp.Body)
		resp.Body.Close()
		message := strings.TrimSpace(string(raw))
		if message == "" {
			message = resp.Status
		}
		return nil, fmt.Errorf("host bridge %s run failed (%d): %s", agent, resp.StatusCode, message)
	}
	return resp.Body, nil
}

func ReadExternalAgentEvents(r io.Reader, emit func(AgentStreamEvent) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event AgentStreamEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return fmt.Errorf("decode host bridge agent event: %w", err)
		}
		if err := emit(event); err != nil {
			return err
		}
	}
	return scanner.Err()
}

type AgentStreamEvent struct {
	Agent    string          `json:"agent"`
	Type     string          `json:"type"`
	Message  string          `json:"message,omitempty"`
	Command  string          `json:"command,omitempty"`
	Files    []string        `json:"files,omitempty"`
	Evidence []string        `json:"evidence,omitempty"`
	Raw      json.RawMessage `json:"raw,omitempty"`
}

func (event AgentStreamEvent) ToOmniEvent(sessionID string) map[string]any {
	out := map[string]any{
		"agent":   firstNonEmpty(event.Agent, "external"),
		"type":    event.Type,
		"message": event.Message,
	}
	if sessionID != "" {
		out["session_id"] = sessionID
	}
	if event.Command != "" {
		out["command"] = event.Command
	}
	if len(event.Files) > 0 {
		out["files"] = event.Files
	}
	if len(event.Evidence) > 0 {
		out["evidence"] = event.Evidence
	}
	if len(event.Raw) > 0 {
		out["raw"] = event.Raw
	}
	return out
}
