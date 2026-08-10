package labyrinth

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func NewScenario(
	id cognition.ScenarioID,
	definition Definition,
	descriptor PublicDescriptor,
) (Scenario, error) {
	if descriptor.ArtifactCorpus != nil {
		return Scenario{}, fmt.Errorf("%w: artifact corpus requires exact private authority", cognition.ErrInvalidScenario)
	}
	return newScenarioWithArtifactCorpus(id, definition, descriptor, nil)
}

func newScenarioWithArtifactCorpus(
	id cognition.ScenarioID,
	definition Definition,
	descriptor PublicDescriptor,
	corpus *artifactCorpus,
) (Scenario, error) {
	definition = definition.clone()
	if err := definition.Validate(); err != nil {
		return Scenario{}, err
	}
	descriptor = descriptor.clone()
	if err := descriptor.Validate(); err != nil {
		return Scenario{}, err
	}
	publicSHA256, err := definition.publicSHA256(id, descriptor)
	if err != nil {
		return Scenario{}, err
	}
	scenario := Scenario{
		ref:              cognition.ScenarioRef{ID: id, SHA256: publicSHA256},
		definition:       definition,
		definitionSHA256: definition.SHA256(),
		descriptor:       descriptor,
		artifactCorpus:   corpus.clone(),
	}
	if err := scenario.Validate(); err != nil {
		return Scenario{}, err
	}
	return scenario, nil
}

func (scenario Scenario) Validate() error {
	if err := scenario.ref.Validate(); err != nil {
		return err
	}
	if err := scenario.definition.Validate(); err != nil {
		return err
	}
	if scenario.definitionSHA256 == "" || scenario.definition.SHA256() != scenario.definitionSHA256 {
		return fmt.Errorf("%w: private definition hash does not bind the sealed world", cognition.ErrInvalidScenario)
	}
	if err := scenario.descriptor.Validate(); err != nil {
		return err
	}
	if err := scenario.artifactCorpus.validate(); err != nil {
		return err
	}
	if (scenario.descriptor.ArtifactCorpus == nil) != (scenario.artifactCorpus == nil) ||
		scenario.artifactCorpus != nil && *scenario.descriptor.ArtifactCorpus != scenario.artifactCorpus.ref {
		return fmt.Errorf("%w: public and private artifact corpus authority differ", cognition.ErrInvalidScenario)
	}
	publicSHA256, err := scenario.definition.publicSHA256(scenario.ref.ID, scenario.descriptor)
	if err != nil {
		return err
	}
	if scenario.ref.SHA256 != publicSHA256 {
		return fmt.Errorf("%w: public hash does not bind the exact public manifest", cognition.ErrInvalidScenario)
	}
	return nil
}

func (scenario Scenario) Ref() cognition.ScenarioRef {
	return scenario.ref
}

func (scenario Scenario) Catalog() cognition.ActionCatalog {
	return scenario.definition.Catalog()
}

// Goal returns the registered objective without exposing the world's latent
// predicates or current fact set.
func (scenario Scenario) Goal() cognition.GoalExpression {
	return scenario.definition.goal.Clone()
}

func (scenario Scenario) MarshalJSON() ([]byte, error) {
	return json.Marshal(scenario.PublicArtifact())
}

func (scenario Scenario) PublicArtifact() GeneratedScenario {
	return GeneratedScenario{
		Schema:   GeneratedScenarioSchemaV1,
		Scenario: scenario.ref,
		World:    scenario.definition.publicManifest(scenario.ref.ID, scenario.descriptor),
	}
}

func (scenario Scenario) action(kind cognition.ActionKind) (ActionDefinition, bool) {
	for _, action := range scenario.definition.actions {
		if action.Schema.Kind == kind {
			return cloneActions([]ActionDefinition{action})[0], true
		}
	}
	return ActionDefinition{}, false
}

func (definition Definition) publicSHA256(
	id cognition.ScenarioID,
	descriptor PublicDescriptor,
) (string, error) {
	digest, _, err := digestJSON(definition.publicManifest(id, descriptor))
	if err != nil {
		return "", fmt.Errorf("%w: encode public scenario manifest: %v", cognition.ErrInvalidScenario, err)
	}
	return digest, nil
}

func (definition Definition) publicManifest(
	id cognition.ScenarioID,
	descriptor PublicDescriptor,
) PublicWorld {
	entities := make([]Entity, 0, len(definition.entities))
	entityIndex := make(map[EntityID]Entity, len(definition.entities))
	for _, entity := range definition.entities {
		entityIndex[entity.ID] = entity
		if entity.Public {
			entities = append(entities, entity)
		}
	}
	predicates := make([]PredicateSchema, 0, len(definition.predicateSchemas))
	predicateIndex := make(map[cognition.PredicateName]PredicateSchema, len(definition.predicateSchemas))
	for _, schema := range definition.predicateSchemas {
		predicateIndex[schema.Name] = schema
		if schema.Public {
			predicates = append(predicates, schema)
		}
	}
	initial := make([]cognition.Predicate, 0, len(definition.initialFacts))
	for _, predicate := range definition.initialFacts {
		if predicateIsPublic(predicate, entityIndex, predicateIndex) {
			initial = append(initial, predicate.Clone())
		}
	}
	return PublicWorld{
		PublicWorldSchemaV1, id, descriptor.clone(), definition.catalog.Clone(), entities, predicates, initial,
	}
}
