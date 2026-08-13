package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	ConversationContextSelectionSchemaV1 = "omnidex.conversation-context-selection.v1"
)

type ConversationContextRole string

const (
	ConversationContextUser      ConversationContextRole = "user"
	ConversationContextAssistant ConversationContextRole = "assistant"
)

type ConversationContextTurn struct {
	MessageID           int64                   `json:"message_id"`
	Role                ConversationContextRole `json:"role"`
	PairedUserMessageID int64                   `json:"paired_user_message_id,omitempty"`
	Content             string                  `json:"content"`
}

type ConversationContextSelectionInput struct {
	ExactInstruction     string                    `json:"exact_instruction"`
	MaxSelectedBytes     int                       `json:"max_selected_bytes"`
	CandidateAuthorities []ConversationContextTurn `json:"candidate_authorities"`
}

type ConversationContextSelectionDecision struct {
	Schema                   string  `json:"schema"`
	ReferencedUserMessageIDs []int64 `json:"referenced_user_message_ids"`
}

func NewConversationContextSelectionJob(input ConversationContextSelectionInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkConversationContextSelection, input, input.validate)
}

func (input ConversationContextSelectionInput) validate() error {
	if err := (ConversationObjectiveKindInput{ExactInstruction: input.ExactInstruction}).validate(); err != nil {
		return err
	}
	if len(input.CandidateAuthorities) < 1 || len(input.CandidateAuthorities) > MaxConversationContextCandidateAuthorities {
		return fmt.Errorf("conversation context selection requires 1..%d candidate authorities", MaxConversationContextCandidateAuthorities)
	}
	if input.MaxSelectedBytes != MaxSelectedConversationProjectionBytes {
		return fmt.Errorf(
			"conversation context selection max_selected_bytes must be %d",
			MaxSelectedConversationProjectionBytes,
		)
	}
	total := 0
	var previousID int64
	users := make(map[int64]struct{}, len(input.CandidateAuthorities))
	pairedUsers := make(map[int64]struct{}, len(input.CandidateAuthorities))
	for index, turn := range input.CandidateAuthorities {
		if turn.MessageID < 1 || turn.MessageID <= previousID {
			return fmt.Errorf("conversation context turn %d has invalid message order", index)
		}
		if turn.Role != ConversationContextUser && turn.Role != ConversationContextAssistant {
			return fmt.Errorf("conversation context turn %d has unsupported role %q", index, turn.Role)
		}
		if turn.Role == ConversationContextUser {
			if turn.PairedUserMessageID != 0 {
				return fmt.Errorf("conversation context user turn %d has an assistant pairing", index)
			}
			users[turn.MessageID] = struct{}{}
		} else {
			if _, ok := users[turn.PairedUserMessageID]; !ok {
				return fmt.Errorf("conversation context assistant turn %d has no preceding paired user", index)
			}
			if _, duplicate := pairedUsers[turn.PairedUserMessageID]; duplicate {
				return fmt.Errorf("conversation context user %d has multiple assistant results", turn.PairedUserMessageID)
			}
			pairedUsers[turn.PairedUserMessageID] = struct{}{}
		}
		if strings.TrimSpace(turn.Content) == "" || !utf8.ValidString(turn.Content) || strings.ContainsRune(turn.Content, '\x00') {
			return fmt.Errorf("conversation context turn %d has invalid exact content", index)
		}
		total += len(turn.Content)
		previousID = turn.MessageID
	}
	if total > MaxConversationContextCandidateBytes {
		return fmt.Errorf("conversation context exceeds %d bytes", MaxConversationContextCandidateBytes)
	}
	return nil
}

func (decision ConversationContextSelectionDecision) ValidateFor(input ConversationContextSelectionInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != ConversationContextSelectionSchemaV1 {
		return fmt.Errorf("conversation context selection schema must be %q", ConversationContextSelectionSchemaV1)
	}
	if len(decision.ReferencedUserMessageIDs) > MaxConversationContextCandidateAuthorities {
		return fmt.Errorf("conversation context selection exceeds the ID bound")
	}
	available := make(map[int64]struct{}, len(input.CandidateAuthorities))
	for _, turn := range input.CandidateAuthorities {
		if turn.Role == ConversationContextUser {
			available[turn.MessageID] = struct{}{}
		}
	}
	seen := make(map[int64]struct{}, len(decision.ReferencedUserMessageIDs))
	for index, id := range decision.ReferencedUserMessageIDs {
		if _, ok := available[id]; !ok {
			return fmt.Errorf("conversation context reference %d selects unavailable user message %d", index, id)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("conversation context user message %d is duplicated", id)
		}
		seen[id] = struct{}{}
	}
	selectedBytes := 0
	for _, turn := range input.CandidateAuthorities {
		selected := turn.Role == ConversationContextUser
		if turn.Role == ConversationContextAssistant {
			_, selected = seen[turn.PairedUserMessageID]
		} else if selected {
			_, selected = seen[turn.MessageID]
		}
		if selected {
			selectedBytes += len(turn.Content)
		}
	}
	if selectedBytes > input.MaxSelectedBytes {
		return fmt.Errorf(
			"conversation context selection projects %d bytes beyond the %d-byte bound",
			selectedBytes, input.MaxSelectedBytes,
		)
	}
	return nil
}

func DecodeConversationContextSelectionDecision(
	input ConversationContextSelectionInput,
	raw string,
) (ConversationContextSelectionDecision, error) {
	if len(raw) > maxPortableCandidateBytes {
		return ConversationContextSelectionDecision{}, fmt.Errorf(
			"conversation context selection candidate exceeds %d bytes", maxPortableCandidateBytes,
		)
	}
	var decision ConversationContextSelectionDecision
	if err := decodePortablePayload([]byte(raw), &decision); err != nil {
		return ConversationContextSelectionDecision{}, fmt.Errorf(
			"decode conversation context selection decision: %w", err,
		)
	}
	if err := decision.ValidateFor(input); err != nil {
		return ConversationContextSelectionDecision{}, err
	}
	return decision, nil
}

func BuildConversationContextSelectionPrompt(input ConversationContextSelectionInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode conversation context selection: %w", err)
	}
	return strings.Join([]string{
		"Resolve whether the exact current instruction refers to any prior user messages.",
		"Return only prior user message IDs whose exact authority is required to interpret the current instruction. Paired assistant results are projected by code with their user anchors and count toward max_selected_bytes. Return an empty array when no prior user authority is required. Do not classify the objective, answer, plan, or select assistant text.",
		"CONVERSATION_CONTEXT_SELECTION_JSON:\n" + string(projection),
	}, "\n\n"), nil
}

func ConversationContextSelectionResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "referenced_user_message_ids"},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": ConversationContextSelectionSchemaV1},
			"referenced_user_message_ids": map[string]any{
				"type": "array", "maxItems": MaxConversationContextCandidateAuthorities,
				"uniqueItems": true, "items": map[string]any{"type": "integer", "minimum": 1},
			},
		},
	)
}
