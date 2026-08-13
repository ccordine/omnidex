package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

const MemoryContextSelectionSchemaV1 = "omnidex.memory-context-selection.v1"

type MemoryContextCandidate struct {
	MemoryID      int64            `json:"memory_id"`
	Kind          model.MemoryKind `json:"kind"`
	Content       string           `json:"content"`
	ContentSHA256 string           `json:"content_sha256"`
}

type MemoryContextSelectionInput struct {
	ExactInstruction     string                   `json:"exact_instruction"`
	MaxSelectedBytes     int                      `json:"max_selected_bytes"`
	CandidateAuthorities []MemoryContextCandidate `json:"candidate_authorities"`
}

type MemoryContextSelectionDecision struct {
	Schema              string  `json:"schema"`
	ReferencedMemoryIDs []int64 `json:"referenced_memory_ids"`
}

func NewMemoryContextSelectionJob(input MemoryContextSelectionInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkMemoryContextSelection, input, input.validate)
}

func (input MemoryContextSelectionInput) validate() error {
	if err := (ConversationObjectiveKindInput{ExactInstruction: input.ExactInstruction}).validate(); err != nil {
		return err
	}
	if len(input.CandidateAuthorities) < 1 ||
		len(input.CandidateAuthorities) > MaxMemoryContextCandidateAuthorities {
		return fmt.Errorf("memory context selection requires 1..%d candidate authorities", MaxMemoryContextCandidateAuthorities)
	}
	if input.MaxSelectedBytes != MaxSelectedMemoryProjectionBytes {
		return fmt.Errorf("memory context max_selected_bytes must be %d", MaxSelectedMemoryProjectionBytes)
	}
	seen := make(map[int64]struct{}, len(input.CandidateAuthorities))
	total := 0
	for index, candidate := range input.CandidateAuthorities {
		if candidate.MemoryID < 1 {
			return fmt.Errorf("memory context candidate %d has invalid identity", index)
		}
		if _, duplicate := seen[candidate.MemoryID]; duplicate {
			return fmt.Errorf("memory context candidate %d is duplicated", candidate.MemoryID)
		}
		seen[candidate.MemoryID] = struct{}{}
		if _, err := model.ParseMemoryKind(string(candidate.Kind)); err != nil {
			return fmt.Errorf("memory context candidate %d: %w", index, err)
		}
		if err := validateObjectiveContextText("memory candidate content", candidate.Content, model.MaxMemoryContentBytes); err != nil {
			return fmt.Errorf("memory context candidate %d: %w", index, err)
		}
		if !exactObjectiveContextSHA(candidate.Content, candidate.ContentSHA256) {
			return fmt.Errorf("memory context candidate %d content hash does not match", index)
		}
		total += len(candidate.Content)
	}
	if total > MaxMemoryContextCandidateBytes {
		return fmt.Errorf("memory context candidates exceed %d bytes", MaxMemoryContextCandidateBytes)
	}
	return nil
}

func (decision MemoryContextSelectionDecision) ValidateFor(input MemoryContextSelectionInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != MemoryContextSelectionSchemaV1 {
		return fmt.Errorf("memory context selection schema must be %q", MemoryContextSelectionSchemaV1)
	}
	if len(decision.ReferencedMemoryIDs) > len(input.CandidateAuthorities) {
		return fmt.Errorf("memory context selection exceeds the candidate ID bound")
	}
	available := make(map[int64]MemoryContextCandidate, len(input.CandidateAuthorities))
	for _, candidate := range input.CandidateAuthorities {
		available[candidate.MemoryID] = candidate
	}
	seen := make(map[int64]struct{}, len(decision.ReferencedMemoryIDs))
	selectedBytes := 0
	for index, id := range decision.ReferencedMemoryIDs {
		candidate, exists := available[id]
		if !exists {
			return fmt.Errorf("memory context reference %d selects unavailable memory %d", index, id)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("memory context reference %d is duplicated", id)
		}
		seen[id] = struct{}{}
		selectedBytes += len(candidate.Content)
	}
	if selectedBytes > input.MaxSelectedBytes {
		return fmt.Errorf("memory context selection projects %d bytes beyond the %d-byte bound", selectedBytes, input.MaxSelectedBytes)
	}
	return nil
}

func DecodeMemoryContextSelectionDecision(
	input MemoryContextSelectionInput,
	raw string,
) (MemoryContextSelectionDecision, error) {
	if len(raw) > maxPortableCandidateBytes {
		return MemoryContextSelectionDecision{}, fmt.Errorf("memory context selection candidate exceeds %d bytes", maxPortableCandidateBytes)
	}
	var decision MemoryContextSelectionDecision
	if err := decodePortablePayload([]byte(raw), &decision); err != nil {
		return MemoryContextSelectionDecision{}, fmt.Errorf("decode memory context selection decision: %w", err)
	}
	if err := decision.ValidateFor(input); err != nil {
		return MemoryContextSelectionDecision{}, err
	}
	return decision, nil
}

func BuildMemoryContextSelectionPrompt(input MemoryContextSelectionInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode memory context selection: %w", err)
	}
	return strings.Join([]string{
		"Select only memory IDs whose exact content is relevant to interpreting or answering the exact current instruction.",
		"Return IDs only. Return an empty array when none are relevant. Do not rewrite memory, infer new facts, classify the objective, answer, plan, retrieve more history, or decide how memory works.",
		"MEMORY_CONTEXT_SELECTION_JSON:\n" + string(projection),
	}, "\n\n"), nil
}

func MemoryContextSelectionResponseSchema() map[string]any {
	return objectSchema(
		[]string{"schema", "referenced_memory_ids"},
		map[string]any{
			"schema": map[string]any{"type": "string", "const": MemoryContextSelectionSchemaV1},
			"referenced_memory_ids": map[string]any{
				"type": "array", "maxItems": MaxMemoryContextCandidateAuthorities,
				"uniqueItems": true, "items": map[string]any{"type": "integer", "minimum": 1},
			},
		},
	)
}
