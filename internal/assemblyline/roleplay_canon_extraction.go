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
	Source             RoleplayCanonSource      `json:"source"`
	AntecedentUserTurn *RoleplayCanonAntecedent `json:"antecedent_user_turn,omitempty"`
	Context            ObjectiveContext         `json:"context"`
}

type RoleplayCanonExtractionDecision struct {
	Schema string   `json:"schema"`
	Facts  []string `json:"facts"`
}

func NewRoleplayCanonExtractionJob(input RoleplayCanonExtractionInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkRoleplayCanonExtraction, input, input.validate)
}

func (input RoleplayCanonExtractionInput) validate() error {
	if err := input.Source.validate(); err != nil {
		return err
	}
	switch input.Source.Kind {
	case RoleplayCanonSourceUserContribution:
		if input.AntecedentUserTurn != nil {
			return fmt.Errorf("roleplay user canon source cannot carry an antecedent user turn")
		}
	case RoleplayCanonSourceAssistantResponse:
		if input.AntecedentUserTurn == nil {
			return fmt.Errorf("roleplay assistant canon source requires its typed antecedent user turn")
		}
		if err := input.AntecedentUserTurn.validate(); err != nil {
			return err
		}
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
	for _, fact := range decision.Facts {
		if err := roleplay.ValidateCanonFact(fact); err != nil {
			return RoleplayCanonExtractionDecision{}, err
		}
	}
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
		Source             RoleplayCanonSource             `json:"source"`
		AntecedentUserTurn *RoleplayCanonAntecedent        `json:"antecedent_user_turn,omitempty"`
		Context            objectiveContextModelProjection `json:"context"`
	}{
		Source: input.Source, AntecedentUserTurn: input.AntecedentUserTurn,
		Context: modelContext,
	})
	if err != nil {
		return "", fmt.Errorf("encode roleplay canon extraction input: %w", err)
	}
	return strings.Join([]string{
		"Extract up to eight newly established fictional fact candidates from exactly one accepted contribution.",
		"Treat only source.exact_contribution as the candidate fact source. Context is established reference material and must never be returned as a newly established fact.",
		"When antecedent_user_turn is present, use it only to resolve references in the assistant contribution. Never extract a fact from the antecedent user turn.",
		"Questions, requests, and directions are not themselves fictional events.",
		"Attribute every first-person statement, action, possession, or item of knowledge in the contribution only to " + strconv.Quote(input.Source.AttributedPersonaName) + ".",
		"Return zero to eight concise standalone fictional fact strings, excluding implications, restatements of known facts, inferred character visibility, and real-world claims.",
		"Return an empty fact array when this exact contribution establishes no new durable fictional fact.",
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
