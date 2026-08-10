package labyrinth

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

const PrivateScenarioSchemaV1 = "labyrinth.private-scenario.v1"

type privateDefinitionPayload struct {
	Format           string                   `json:"format"`
	Catalog          cognition.ActionCatalog  `json:"catalog"`
	Entities         []Entity                 `json:"entities"`
	PredicateSchemas []PredicateSchema        `json:"predicate_schemas"`
	InitialFacts     []cognition.Predicate    `json:"initial_facts"`
	Actions          []ActionDefinition       `json:"actions"`
	Goal             cognition.GoalExpression `json:"goal"`
}

type privateArtifactCorpusPayload struct {
	Seed   uint64            `json:"seed"`
	Stages []EntityID        `json:"stages"`
	Ref    ArtifactCorpusRef `json:"ref"`
}

type privateScenarioPayload struct {
	Schema           string                        `json:"schema"`
	Reference        cognition.ScenarioRef         `json:"reference"`
	Definition       privateDefinitionPayload      `json:"definition"`
	DefinitionSHA256 string                        `json:"definition_sha256"`
	Descriptor       PublicDescriptor              `json:"descriptor"`
	ArtifactCorpus   *privateArtifactCorpusPayload `json:"artifact_corpus"`
}

// MarshalPrivateJSON emits host-only world authority. It must never be passed
// to an inference process or exposed by an environment observation.
func (scenario Scenario) MarshalPrivateJSON() ([]byte, error) {
	if err := scenario.Validate(); err != nil {
		return nil, err
	}
	payload := privateScenarioPayload{
		Schema: PrivateScenarioSchemaV1, Reference: scenario.ref,
		Definition:       scenario.definition.privatePayload(),
		DefinitionSHA256: scenario.definitionSHA256,
		Descriptor:       scenario.descriptor.clone(),
	}
	if scenario.artifactCorpus != nil {
		payload.ArtifactCorpus = &privateArtifactCorpusPayload{
			Seed:   scenario.artifactCorpus.seed,
			Stages: append([]EntityID(nil), scenario.artifactCorpus.stages...),
			Ref:    scenario.artifactCorpus.ref,
		}
	}
	return json.Marshal(payload)
}

// ParsePrivateScenarioJSON restores one exact host-only world artifact.
func ParsePrivateScenarioJSON(raw []byte) (Scenario, error) {
	var payload privateScenarioPayload
	if err := cognition.ValidateExactJSONObject(raw, &payload, "private Labyrinth scenario"); err != nil {
		return Scenario{}, fmt.Errorf("%w: %v", cognition.ErrInvalidScenario, err)
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Schema != PrivateScenarioSchemaV1 {
		return Scenario{}, fmt.Errorf("%w: private scenario schema is invalid", cognition.ErrInvalidScenario)
	}
	definition, err := NewDefinition(
		payload.Definition.Catalog, payload.Definition.Entities, payload.Definition.PredicateSchemas,
		payload.Definition.InitialFacts, payload.Definition.Actions, payload.Definition.Goal,
	)
	if err != nil || payload.Definition.Format != definitionFormat ||
		definition.SHA256() != payload.DefinitionSHA256 {
		return Scenario{}, fmt.Errorf("%w: private definition authority is invalid", cognition.ErrInvalidScenario)
	}
	corpus, err := restorePrivateArtifactCorpus(payload.ArtifactCorpus)
	if err != nil {
		return Scenario{}, err
	}
	scenario, err := newScenarioWithArtifactCorpus(
		payload.Reference.ID, definition, payload.Descriptor, corpus,
	)
	if err != nil || scenario.ref != payload.Reference {
		return Scenario{}, fmt.Errorf("%w: private scenario reference is invalid", cognition.ErrInvalidScenario)
	}
	return scenario, nil
}

func (definition Definition) privatePayload() privateDefinitionPayload {
	return privateDefinitionPayload{
		Format: definitionFormat, Catalog: definition.catalog.Clone(),
		Entities:         cloneEntities(definition.entities),
		PredicateSchemas: clonePredicateSchemas(definition.predicateSchemas),
		InitialFacts:     clonePredicates(definition.initialFacts),
		Actions:          cloneActions(definition.actions), Goal: definition.goal.Clone(),
	}
}

func restorePrivateArtifactCorpus(payload *privateArtifactCorpusPayload) (*artifactCorpus, error) {
	if payload == nil {
		return nil, nil
	}
	corpus, err := newArtifactCorpus(payload.Seed, payload.Ref.Count, payload.Stages)
	if err != nil || corpus.ref != payload.Ref {
		return nil, fmt.Errorf("%w: private artifact corpus authority is invalid", cognition.ErrInvalidScenario)
	}
	return corpus, nil
}
