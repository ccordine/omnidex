package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/roleplay"
)

const (
	RoleplayCanonExtractionSchemaV1 = "omnidex.roleplay-canon-extraction.v1"
	MaxRoleplayCanonFactsPerTurn    = roleplay.MaxCanonFactsPerTurn
)

type RoleplayCanonExtractionInput struct {
	ExactInstruction  string   `json:"exact_instruction"`
	AssistantResponse string   `json:"assistant_response"`
	KnownFacts        []string `json:"known_facts"`
}

type RoleplayCanonExtractionDecision struct {
	Schema string   `json:"schema"`
	Facts  []string `json:"facts"`
}

func NewRoleplayCanonExtractionJob(input RoleplayCanonExtractionInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkRoleplayCanonExtraction, input, input.validate)
}

func (input RoleplayCanonExtractionInput) validate() error {
	if err := validateGroundedText(
		"roleplay exact instruction", input.ExactInstruction, maxConversationInstructionBytes, false,
	); err != nil {
		return err
	}
	if err := validateGroundedText(
		"roleplay assistant response", input.AssistantResponse, maxConversationResponseTextBytes, true,
	); err != nil {
		return err
	}
	if len(input.KnownFacts) > roleplay.MaxProjectionEvents {
		return fmt.Errorf("roleplay known-fact projection exceeds its bound")
	}
	seen := make(map[string]struct{}, len(input.KnownFacts))
	for _, fact := range input.KnownFacts {
		if err := roleplay.ValidateCanonFact(fact); err != nil {
			return err
		}
		if _, duplicate := seen[fact]; duplicate {
			return fmt.Errorf("roleplay known fact is duplicated")
		}
		seen[fact] = struct{}{}
	}
	return nil
}

func (decision RoleplayCanonExtractionDecision) ValidateFor(
	input RoleplayCanonExtractionInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != RoleplayCanonExtractionSchemaV1 {
		return fmt.Errorf("roleplay canon extraction schema must be %q", RoleplayCanonExtractionSchemaV1)
	}
	if decision.Facts == nil || len(decision.Facts) > MaxRoleplayCanonFactsPerTurn {
		return fmt.Errorf("roleplay canon extraction facts must be an explicit bounded array")
	}
	known := make(map[string]struct{}, len(input.KnownFacts))
	for _, fact := range input.KnownFacts {
		known[fact] = struct{}{}
	}
	seen := make(map[string]struct{}, len(decision.Facts))
	for _, fact := range decision.Facts {
		if err := roleplay.ValidateCanonFact(fact); err != nil {
			return err
		}
		if _, exists := known[fact]; exists {
			return fmt.Errorf("roleplay canon extraction repeated an established fact")
		}
		if _, duplicate := seen[fact]; duplicate {
			return fmt.Errorf("roleplay canon extraction duplicated a fact")
		}
		seen[fact] = struct{}{}
	}
	return nil
}

func DecodeRoleplayCanonExtractionDecision(
	input RoleplayCanonExtractionInput,
	raw string,
) (RoleplayCanonExtractionDecision, error) {
	var decision RoleplayCanonExtractionDecision
	if len(raw) > maxPortableCandidateBytes {
		return decision, fmt.Errorf("roleplay canon extraction candidate exceeds %d bytes", maxPortableCandidateBytes)
	}
	if err := decodePortablePayload([]byte(raw), &decision); err != nil {
		return decision, fmt.Errorf("decode roleplay canon extraction: %w", err)
	}
	if err := decision.ValidateFor(input); err != nil {
		return decision, err
	}
	return decision, nil
}

func BuildRoleplayCanonExtractionPrompt(input RoleplayCanonExtractionInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode roleplay canon extraction input: %w", err)
	}
	return strings.Join([]string{
		"Extract only the newly established fictional facts in one assistant narrative response.",
		"Return zero to eight concise standalone fictional fact strings, excluding implications, restatements of known facts, character visibility, and real-world claims.",
		"ROLEPLAY_CANON_EXTRACTION_JSON:\n" + string(projection),
	}, "\n\n"), nil
}

func RoleplayCanonExtractionResponseSchema() map[string]any {
	return objectSchema([]string{"schema", "facts"}, map[string]any{
		"schema": map[string]any{"type": "string", "const": RoleplayCanonExtractionSchemaV1},
		"facts": map[string]any{
			"type": "array", "minItems": 0, "maxItems": MaxRoleplayCanonFactsPerTurn,
			"items": map[string]any{
				"type": "string", "minLength": 1, "maxLength": roleplay.MaxCanonEventBytes,
			},
		},
	})
}
