package assemblyline

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/roleplay"
)

const (
	RoleplayCanonExtractionSchemaV1 = "omnidex.roleplay-canon-extraction.v1"
	MaxRoleplayCanonFactsPerTurn    = roleplay.MaxCanonFactsPerTurn
)

type RoleplayCanonExtractionInput struct {
	ExactInstruction        string                     `json:"exact_instruction"`
	AssistantResponse       string                     `json:"assistant_response"`
	RespondingCharacterName string                     `json:"responding_character_name"`
	Context                 ObjectiveContext           `json:"context"`
	UserTurn                RoleplayUserTurnProjection `json:"user_turn"`
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
	if err := validateContextText(
		"roleplay responding character name", input.RespondingCharacterName, 256,
	); err != nil {
		return err
	}
	if err := input.UserTurn.validate(); err != nil {
		return err
	}
	return input.Context.Validate()
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
	if decision.Facts == nil {
		return fmt.Errorf("roleplay canon extraction facts must be an explicit array")
	}
	if len(decision.Facts) > MaxRoleplayCanonFactsPerTurn {
		return fmt.Errorf(
			"roleplay canon extraction facts must contain 0..%d current-turn facts",
			MaxRoleplayCanonFactsPerTurn,
		)
	}
	seen := make(map[string]struct{}, len(decision.Facts))
	for _, fact := range decision.Facts {
		if err := roleplay.ValidateCanonFact(fact); err != nil {
			return err
		}
		if _, duplicate := seen[fact]; duplicate {
			return fmt.Errorf("roleplay canon extraction duplicated a fact")
		}
		seen[fact] = struct{}{}
	}
	return nil
}

func (decision RoleplayCanonExtractionDecision) ResolveFor(
	input RoleplayCanonExtractionInput,
) (RoleplayCanonExtractionDecision, error) {
	if err := input.validate(); err != nil {
		return RoleplayCanonExtractionDecision{}, err
	}
	if decision.Schema != RoleplayCanonExtractionSchemaV1 {
		return RoleplayCanonExtractionDecision{}, fmt.Errorf(
			"roleplay canon extraction schema must be %q", RoleplayCanonExtractionSchemaV1,
		)
	}
	if decision.Facts == nil {
		return RoleplayCanonExtractionDecision{}, fmt.Errorf(
			"roleplay canon extraction facts must be an explicit array",
		)
	}
	if len(decision.Facts) > MaxRoleplayCanonFactsPerTurn {
		return RoleplayCanonExtractionDecision{}, fmt.Errorf(
			"roleplay canon extraction facts must contain 0..%d current-turn facts",
			MaxRoleplayCanonFactsPerTurn,
		)
	}
	seen := make(map[string]struct{}, len(decision.Facts))
	resolved := make([]string, 0, len(decision.Facts))
	for _, fact := range decision.Facts {
		if err := roleplay.ValidateCanonFact(fact); err != nil {
			return RoleplayCanonExtractionDecision{}, err
		}
		if _, duplicate := seen[fact]; duplicate {
			continue
		}
		seen[fact] = struct{}{}
		resolved = append(resolved, fact)
	}
	decision.Facts = resolved
	if err := decision.ValidateFor(input); err != nil {
		return RoleplayCanonExtractionDecision{}, err
	}
	return decision, nil
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
	return decision.ResolveFor(input)
}

func BuildRoleplayCanonExtractionPrompt(input RoleplayCanonExtractionInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	modelContext, err := projectObjectiveContextForModel(input.Context)
	if err != nil {
		return "", err
	}
	projection, err := json.Marshal(struct {
		ExactInstruction        string                          `json:"exact_instruction"`
		AssistantResponse       string                          `json:"assistant_response"`
		RespondingCharacterName string                          `json:"responding_character_name"`
		UserTurn                RoleplayUserTurnProjection      `json:"user_turn"`
		Context                 objectiveContextModelProjection `json:"context"`
	}{
		ExactInstruction: input.ExactInstruction, AssistantResponse: input.AssistantResponse,
		RespondingCharacterName: input.RespondingCharacterName,
		UserTurn:                input.UserTurn, Context: modelContext,
	})
	if err != nil {
		return "", fmt.Errorf("encode roleplay canon extraction input: %w", err)
	}
	return strings.Join([]string{
		"Extract up to eight newly established fictional fact candidates from one complete accepted turn, using the exact user turn and final assistant narrative together.",
		"Facts may come from explicit fictional actions or assertions in either exact_instruction or assistant_response. Questions and requests are not themselves fictional events.",
		"Attribute the exact user contribution to " + strconv.Quote(input.UserTurn.PersonaName) + " and the assistant response to " + strconv.Quote(input.RespondingCharacterName) + ". Never transfer first-person speech, actions, possessions, or knowledge between them.",
		"Return zero to eight concise standalone fictional fact strings, excluding implications, restatements of known facts, inferred character visibility, and real-world claims.",
		"Return an empty fact array when the accepted turn establishes no new durable fictional fact.",
		"Prefer participant actions, possessions, relationships, promises, explicit knowledge, and scene changes over decorative sensory descriptions when the bound requires selection.",
		"ROLEPLAY_CANON_EXTRACTION_JSON:\n" + string(projection),
	}, "\n\n"), nil
}

func RoleplayCanonExtractionResponseSchema() map[string]any {
	return objectSchema([]string{"schema", "facts"}, map[string]any{
		"schema": map[string]any{"type": "string", "const": RoleplayCanonExtractionSchemaV1},
		"facts": map[string]any{
			"type": "array", "minItems": 0, "maxItems": MaxRoleplayCanonFactsPerTurn,
			"uniqueItems": true,
			"items": map[string]any{
				"type": "string", "minLength": 1, "maxLength": roleplay.MaxCanonEventBytes,
			},
		},
	})
}
