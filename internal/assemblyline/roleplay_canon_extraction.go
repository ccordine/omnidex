package assemblyline

import (
	"fmt"

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
