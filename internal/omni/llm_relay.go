package omni

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const defaultLLMRelayTimeout = 30 * time.Second

var relayRoleIDRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,63}$`)

type LLMRelayService struct {
	client  *OllamaClient
	timeout time.Duration
}

type RelayMessage struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Payload  string `json:"payload"`
	Checksum string `json:"checksum"`
}

type RelayHop struct {
	FromRole string
	ToRole   string
	Message  RelayMessage
	Raw      string
}

type TelephoneGameResult struct {
	Hops         []RelayHop
	Final        RelayMessage
	Delivered    bool
	FinalPayload string
}

func NewLLMRelayService(client *OllamaClient) *LLMRelayService {
	return &LLMRelayService{client: client, timeout: defaultLLMRelayTimeout}
}

func (s *LLMRelayService) WithTimeout(timeout time.Duration) *LLMRelayService {
	if timeout > 0 {
		s.timeout = timeout
	}
	return s
}

func (s *LLMRelayService) Send(ctx context.Context, fromRole, toRole, payload string) (RelayHop, error) {
	fromRole = strings.TrimSpace(fromRole)
	toRole = strings.TrimSpace(toRole)
	payload = strings.TrimSpace(payload)
	if !relayRoleIDRe.MatchString(fromRole) {
		return RelayHop{}, fmt.Errorf("invalid from role %q", fromRole)
	}
	if !relayRoleIDRe.MatchString(toRole) {
		return RelayHop{}, fmt.Errorf("invalid to role %q", toRole)
	}
	if payload == "" {
		return RelayHop{}, fmt.Errorf("relay payload cannot be empty")
	}
	if s == nil || s.client == nil {
		return RelayHop{}, fmt.Errorf("relay requires an Ollama client")
	}

	checksum := RelayChecksum(payload)
	callCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	resp, err := s.client.ChatRaw(callCtx, OllamaChatRequest{
		Messages: []OllamaMessage{
			{Role: "system", Content: buildRelaySystemPrompt(fromRole, toRole, checksum)},
			{Role: "user", Content: payload},
		},
		Options: map[string]interface{}{"temperature": 0, "num_predict": 300},
	})
	if err != nil {
		return RelayHop{}, err
	}

	message, err := ParseRelayMessage(resp.Content)
	if err != nil {
		return RelayHop{}, err
	}
	if message.From != fromRole {
		return RelayHop{}, fmt.Errorf("relay from mismatch: got %q want %q", message.From, fromRole)
	}
	if message.To != toRole {
		return RelayHop{}, fmt.Errorf("relay to mismatch: got %q want %q", message.To, toRole)
	}
	if message.Checksum != checksum {
		return RelayHop{}, fmt.Errorf("relay checksum mismatch: got %q want %q", message.Checksum, checksum)
	}
	if message.Payload != payload {
		return RelayHop{}, fmt.Errorf("relay payload mismatch")
	}

	return RelayHop{
		FromRole: fromRole,
		ToRole:   toRole,
		Message:  message,
		Raw:      resp.Content,
	}, nil
}

func (s *LLMRelayService) TelephoneGame(ctx context.Context, roles []string, payload string) (TelephoneGameResult, error) {
	if len(roles) < 2 {
		return TelephoneGameResult{}, fmt.Errorf("telephone game requires at least two roles")
	}
	currentPayload := strings.TrimSpace(payload)
	if currentPayload == "" {
		return TelephoneGameResult{}, fmt.Errorf("telephone game payload cannot be empty")
	}

	result := TelephoneGameResult{Hops: make([]RelayHop, 0, len(roles)-1)}
	for i := 0; i < len(roles)-1; i++ {
		hop, err := s.Send(ctx, roles[i], roles[i+1], currentPayload)
		if err != nil {
			return result, err
		}
		result.Hops = append(result.Hops, hop)
		currentPayload = hop.Message.Payload
	}

	result.Final = result.Hops[len(result.Hops)-1].Message
	result.FinalPayload = result.Final.Payload
	result.Delivered = result.FinalPayload == strings.TrimSpace(payload) && result.Final.Checksum == RelayChecksum(payload)
	if !result.Delivered {
		return result, fmt.Errorf("telephone game delivery failed")
	}
	return result, nil
}

func RelayChecksum(payload string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(payload)))
	return hex.EncodeToString(sum[:])
}

func ParseRelayMessage(raw string) (RelayMessage, error) {
	clean := strings.TrimSpace(raw)
	clean = strings.TrimPrefix(clean, "```json")
	clean = strings.TrimPrefix(clean, "```")
	clean = strings.TrimSuffix(clean, "```")
	clean = strings.TrimSpace(clean)

	var msg RelayMessage
	if err := json.Unmarshal([]byte(clean), &msg); err != nil {
		return RelayMessage{}, fmt.Errorf("parse relay message JSON: %w", err)
	}
	msg.From = strings.TrimSpace(msg.From)
	msg.To = strings.TrimSpace(msg.To)
	msg.Payload = strings.TrimSpace(msg.Payload)
	msg.Checksum = strings.TrimSpace(msg.Checksum)
	if msg.From == "" || msg.To == "" || msg.Payload == "" || msg.Checksum == "" {
		return RelayMessage{}, fmt.Errorf("relay message missing required fields")
	}
	return msg, nil
}

func buildRelaySystemPrompt(fromRole, toRole, checksum string) string {
	return strings.Join(withMinimalOutputContract(
		"Role: relay.",
		"Transmit payload exactly.",
		"Output JSON only. No markdown. No prose.",
		"Schema: {\"from\":\"...\",\"to\":\"...\",\"payload\":\"...\",\"checksum\":\"...\"}",
		"No summarize/rephrase/correct/expand/omit.",
		fmt.Sprintf("from must be %q.", fromRole),
		fmt.Sprintf("to must be %q.", toRole),
		fmt.Sprintf("checksum must be %q.", checksum),
	), "\n")
}
