package labyrinth

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
)

func GenerateScaleFamily(
	base GeneratorConfig,
	worldSizes []int,
) ([]GeneratedCase, ScaleFamilyDescriptor, error) {
	if err := validateScaleCoordinates(base, worldSizes); err != nil {
		return nil, ScaleFamilyDescriptor{}, err
	}
	baseCase, err := Generate(base)
	if err != nil {
		return nil, ScaleFamilyDescriptor{}, err
	}
	relevantSHA, goalSHA, err := scaleRelevantAuthority(baseCase)
	if err != nil {
		return nil, ScaleFamilyDescriptor{}, err
	}
	familyID, _, err := digestJSON(struct {
		Schema      string          `json:"schema"`
		Base        GeneratorConfig `json:"base"`
		RelevantSHA string          `json:"relevant_sha256"`
	}{ScaleFamilySchemaV1, base, relevantSHA})
	if err != nil {
		return nil, ScaleFamilyDescriptor{}, err
	}
	family := ScaleFamilyDescriptor{
		Schema: ScaleFamilySchemaV1, FamilyID: "scale-family-" + familyID,
		GeneratorVersion: base.GeneratorVersion, GrammarVersion: base.GrammarVersion,
		Suite: base.Suite, RelevantSurfaceSHA256: relevantSHA, GoalSHA256: goalSHA,
		ActionCatalog: baseCase.ExecutionScenario().Catalog(),
		Cases:         make([]ScaleFamilyCase, 0, len(worldSizes)),
	}
	cases := make([]GeneratedCase, 0, len(worldSizes))
	for _, size := range worldSizes {
		generated, buildErr := buildScaleCase(baseCase, base.Seed, family.FamilyID, size)
		if buildErr != nil {
			return nil, ScaleFamilyDescriptor{}, buildErr
		}
		caseRelevant, caseGoal, buildErr := scaleRelevantAuthority(generated)
		if buildErr != nil || caseRelevant != relevantSHA || caseGoal != goalSHA ||
			!reflect.DeepEqual(generated.ExecutionScenario().Catalog(), family.ActionCatalog) {
			return nil, ScaleFamilyDescriptor{}, fmt.Errorf("%w: scale case changed the relevant surface", ErrGeneration)
		}
		if transition, _, witnessErr := VerifyWitness(generated); witnessErr != nil || !transition.Terminal {
			return nil, ScaleFamilyDescriptor{}, fmt.Errorf("%w: scale witness failed: %v", ErrGeneration, witnessErr)
		}
		cases = append(cases, generated)
		family.Cases = append(family.Cases, ScaleFamilyCase{
			Scenario: generated.PublicArtifact().Scenario, WorldSize: size,
		})
	}
	if err := family.Validate(); err != nil {
		return nil, ScaleFamilyDescriptor{}, err
	}
	return cases, family.clone(), nil
}

func validateScaleCoordinates(base GeneratorConfig, sizes []int) error {
	if err := base.Validate(); err != nil {
		return err
	}
	if len(sizes) < 2 || sizes[0] != base.Difficulty.WorldSize {
		return fmt.Errorf("%w: scale sizes must start at the base world", ErrInvalidGeneratorConfig)
	}
	for index, size := range sizes {
		if size < base.Difficulty.WorldSize || size > MaxScaleWorldSize || index > 0 && size <= sizes[index-1] {
			return fmt.Errorf("%w: scale sizes must be unique, increasing, and bounded", ErrInvalidGeneratorConfig)
		}
	}
	if sizes[len(sizes)-1] < sizes[0]*100 {
		return fmt.Errorf("%w: scale family must include at least a 100x world", ErrInvalidGeneratorConfig)
	}
	return nil
}

func buildScaleCase(base GeneratedCase, seed uint64, familyID string, size int) (GeneratedCase, error) {
	scenario := base.ExecutionScenario()
	descriptor := scenario.descriptor.clone()
	baseCount := len(descriptor.Records)
	if size < baseCount {
		return GeneratedCase{}, fmt.Errorf("%w: scale world is smaller than its fixed surface", ErrGeneration)
	}
	descriptor.Difficulty.WorldSize = size
	var corpus *artifactCorpus
	if size > baseCount {
		var err error
		corpus, err = newArtifactCorpus(seed, size-baseCount, scenarioStageIDs(scenario))
		if err != nil {
			return GeneratedCase{}, err
		}
		ref := corpus.ref
		descriptor.ArtifactCorpus = &ref
	} else {
		descriptor.ArtifactCorpus = nil
	}
	digest, _, err := digestJSON(struct {
		Schema   string             `json:"schema"`
		FamilyID string             `json:"family_id"`
		Size     int                `json:"world_size"`
		Corpus   *ArtifactCorpusRef `json:"artifact_corpus,omitempty"`
	}{ScaleFamilySchemaV1, familyID, size, descriptor.ArtifactCorpus})
	if err != nil {
		return GeneratedCase{}, err
	}
	scaledScenario, err := newScenarioWithArtifactCorpus(
		cognition.ScenarioID("scenario-"+digest), scenario.definition, descriptor, corpus,
	)
	if err != nil {
		return GeneratedCase{}, err
	}
	oracle := base.PrivateOracle()
	oracle.ScenarioID = scaledScenario.Ref().ID
	oracle.PublicSHA256 = scaledScenario.Ref().SHA256
	oracle.OracleSHA256 = ""
	if err := oracle.seal(); err != nil {
		return GeneratedCase{}, err
	}
	generated := GeneratedCase{
		execution: scaledScenario, public: scaledScenario.PublicArtifact(), oracle: oracle,
	}
	if err := generated.Validate(); err != nil {
		return GeneratedCase{}, err
	}
	return generated, nil
}

func scaleRelevantAuthority(generated GeneratedCase) (string, string, error) {
	oracle := generated.PrivateOracle()
	scenario := generated.ExecutionScenario()
	records := make(map[string]PublicRecord, len(scenario.descriptor.Records))
	for _, record := range scenario.descriptor.Records {
		records[string(record.ID)] = record
	}
	relevant := make([]PublicRecord, len(oracle.RequiredEvidence))
	for index, evidence := range oracle.RequiredEvidence {
		record, exists := records[evidence.ID]
		if !exists || record.ContentSHA256 != evidence.SHA256 {
			return "", "", fmt.Errorf("%w: scale evidence is absent", ErrGeneration)
		}
		relevant[index] = record
	}
	goalSHA, _, err := digestJSON(scenario.Goal())
	if err != nil {
		return "", "", err
	}
	relevantSHA, _, err := digestJSON(struct {
		Goal       cognition.GoalExpression `json:"goal"`
		Catalog    cognition.ActionCatalog  `json:"catalog"`
		Witness    []WitnessAction          `json:"witness"`
		Evidence   []EvidenceIdentity       `json:"evidence"`
		Uses       []EvidenceUse            `json:"uses"`
		Records    []PublicRecord           `json:"records"`
		Definition string                   `json:"definition_sha256"`
	}{scenario.Goal(), scenario.Catalog(), oracle.Witness, oracle.RequiredEvidence,
		oracle.EvidenceUses, relevant, oracle.DefinitionSHA256})
	return relevantSHA, goalSHA, err
}

func scenarioStageIDs(scenario Scenario) []EntityID {
	stages := make([]EntityID, 0)
	for _, entity := range scenario.definition.entities {
		if entity.Kind == stageKind {
			stages = append(stages, entity.ID)
		}
	}
	return stages
}
