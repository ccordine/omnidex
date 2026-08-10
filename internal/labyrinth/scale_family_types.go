package labyrinth

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

const ScaleFamilySchemaV1 = "labyrinth.scale-family.v1"

type ScaleFamilyCase struct {
	Scenario  cognition.ScenarioRef `json:"scenario"`
	WorldSize int                   `json:"world_size"`
}

type ScaleFamilyDescriptor struct {
	Schema                string                  `json:"schema"`
	FamilyID              string                  `json:"family_id"`
	GeneratorVersion      string                  `json:"generator_version"`
	GrammarVersion        string                  `json:"grammar_version"`
	Suite                 Suite                   `json:"suite"`
	RelevantSurfaceSHA256 string                  `json:"relevant_surface_sha256"`
	GoalSHA256            string                  `json:"goal_sha256"`
	ActionCatalog         cognition.ActionCatalog `json:"action_catalog"`
	Cases                 []ScaleFamilyCase       `json:"cases"`
}

func (descriptor ScaleFamilyDescriptor) Validate() error {
	if descriptor.Schema != ScaleFamilySchemaV1 || !validSymbol(descriptor.FamilyID) ||
		descriptor.GeneratorVersion != GeneratorVersionV1 || descriptor.GrammarVersion != GrammarVersionV1 ||
		descriptor.Suite.Validate() != nil || !validDigest(descriptor.RelevantSurfaceSHA256) ||
		!validDigest(descriptor.GoalSHA256) || descriptor.ActionCatalog.Validate() != nil ||
		len(descriptor.Cases) < 2 {
		return fmt.Errorf("%w: scale family authority is invalid", ErrGeneration)
	}
	previous := 0
	seen := make(map[cognition.ScenarioID]struct{}, len(descriptor.Cases))
	for index, item := range descriptor.Cases {
		if item.Scenario.Validate() != nil || item.WorldSize < MinGeneratedWorldSize ||
			item.WorldSize > MaxScaleWorldSize || index > 0 && item.WorldSize <= previous {
			return fmt.Errorf("%w: scale family case %d is invalid", ErrGeneration, index)
		}
		if _, duplicate := seen[item.Scenario.ID]; duplicate {
			return fmt.Errorf("%w: scale family scenario is duplicated", ErrGeneration)
		}
		seen[item.Scenario.ID], previous = struct{}{}, item.WorldSize
	}
	if descriptor.Cases[len(descriptor.Cases)-1].WorldSize < descriptor.Cases[0].WorldSize*100 {
		return fmt.Errorf("%w: scale family does not reach the registered 100x rail", ErrGeneration)
	}
	return nil
}

func (descriptor ScaleFamilyDescriptor) clone() ScaleFamilyDescriptor {
	descriptor.ActionCatalog = descriptor.ActionCatalog.Clone()
	descriptor.Cases = append([]ScaleFamilyCase(nil), descriptor.Cases...)
	return descriptor
}
