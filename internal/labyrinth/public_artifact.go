package labyrinth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const MaxGeneratedArtifactBytes = 256 * 1024

func NewPublicRecord(id, location EntityID, content string) (PublicRecord, error) {
	record := PublicRecord{ID: id, Location: location, Content: content, ContentSHA256: textSHA256(content)}
	if err := record.Validate(); err != nil {
		return PublicRecord{}, err
	}
	return record, nil
}

func (record PublicRecord) Validate() error {
	if !validSymbol(string(record.ID)) || !validSymbol(string(record.Location)) {
		return fmt.Errorf("%w: public record identity is invalid", ErrGeneration)
	}
	if record.Content == "" || len(record.Content) > MaxPublicRecordContentBytes ||
		!utf8.ValidString(record.Content) || strings.ContainsRune(record.Content, 0) ||
		strings.TrimSpace(record.Content) != record.Content {
		return fmt.Errorf("%w: public record content is invalid", ErrGeneration)
	}
	if textSHA256(record.Content) != record.ContentSHA256 {
		return fmt.Errorf("%w: public record hash does not bind exact content", ErrGeneration)
	}
	return nil
}

func (descriptor PublicDescriptor) Validate() error {
	if err := descriptor.Suite.Validate(); err != nil {
		return err
	}
	if err := descriptor.Difficulty.Validate(); err != nil {
		return err
	}
	if !validSymbol(descriptor.FormatVersion) || !validSymbol(descriptor.SurfaceVersion) ||
		!validSymbol(descriptor.GrammarVersion) {
		return fmt.Errorf("%w: public descriptor versions are invalid", ErrGeneration)
	}
	if descriptor.Goal == "" || len(descriptor.Goal) > 2048 || !utf8.ValidString(descriptor.Goal) ||
		strings.ContainsRune(descriptor.Goal, 0) || strings.TrimSpace(descriptor.Goal) != descriptor.Goal {
		return fmt.Errorf("%w: public goal is invalid", ErrGeneration)
	}
	if len(descriptor.Records) > MaxGeneratedWorldSize {
		return fmt.Errorf("%w: public record count exceeds %d", ErrGeneration, MaxGeneratedWorldSize)
	}
	corpusCount := 0
	if descriptor.ArtifactCorpus != nil {
		if err := descriptor.ArtifactCorpus.Validate(); err != nil {
			return err
		}
		corpusCount = descriptor.ArtifactCorpus.Count
		if len(descriptor.Records)+corpusCount != descriptor.Difficulty.WorldSize {
			return fmt.Errorf("%w: public artifacts do not match declared world size", ErrGeneration)
		}
	}
	previous := EntityID("")
	for index, record := range descriptor.Records {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("%w: record %d: %v", ErrGeneration, index, err)
		}
		if index > 0 && record.ID <= previous {
			return fmt.Errorf("%w: public records must be uniquely sorted", ErrGeneration)
		}
		previous = record.ID
	}
	return nil
}

func (descriptor PublicDescriptor) clone() PublicDescriptor {
	descriptor.Records = append([]PublicRecord(nil), descriptor.Records...)
	if descriptor.ArtifactCorpus != nil {
		ref := *descriptor.ArtifactCorpus
		descriptor.ArtifactCorpus = &ref
	}
	return descriptor
}

func (world PublicWorld) Validate() error {
	if world.Schema != PublicWorldSchemaV1 || !validSymbol(string(world.ScenarioID)) {
		return fmt.Errorf("%w: public world identity is invalid", ErrGeneration)
	}
	if err := world.Descriptor.Validate(); err != nil {
		return err
	}
	if err := world.Catalog.Validate(); err != nil {
		return fmt.Errorf("%w: public action catalog: %v", ErrGeneration, err)
	}
	entities, kinds, err := validateEntities(world.Entities)
	if err != nil {
		return err
	}
	for _, entity := range world.Entities {
		if !entity.Public {
			return fmt.Errorf("%w: public world contains a private entity", ErrGeneration)
		}
	}
	predicates, err := validatePredicateSchemas(world.PredicateSchemas, kinds)
	if err != nil {
		return err
	}
	for _, schema := range world.PredicateSchemas {
		if !schema.Public {
			return fmt.Errorf("%w: public world contains a private predicate schema", ErrGeneration)
		}
	}
	if err := validateGroundPredicates(world.InitialFacts, entities, predicates, "public fact"); err != nil {
		return err
	}
	for _, fact := range world.InitialFacts {
		if !predicateIsPublic(fact, entities, predicates) {
			return fmt.Errorf("%w: public world contains a private fact", ErrGeneration)
		}
	}
	for _, record := range world.Descriptor.Records {
		if _, exists := entities[record.ID]; !exists {
			return fmt.Errorf("%w: public record entity is absent", ErrGeneration)
		}
		if _, exists := entities[record.Location]; !exists {
			return fmt.Errorf("%w: public record location is absent", ErrGeneration)
		}
	}
	return nil
}

func (artifact GeneratedScenario) Validate() error {
	if artifact.Schema != GeneratedScenarioSchemaV1 {
		return fmt.Errorf("%w: generated scenario schema is invalid", ErrGeneration)
	}
	if err := artifact.Scenario.Validate(); err != nil {
		return err
	}
	if err := artifact.World.Validate(); err != nil {
		return err
	}
	if artifact.Scenario.ID != artifact.World.ScenarioID {
		return fmt.Errorf("%w: scenario and public world identities differ", ErrGeneration)
	}
	digest, _, err := digestJSON(artifact.World)
	if err != nil || digest != artifact.Scenario.SHA256 {
		return fmt.Errorf("%w: scenario hash does not bind exact public world", ErrGeneration)
	}
	return nil
}

func (artifact GeneratedScenario) MarshalJSON() ([]byte, error) {
	if err := artifact.Validate(); err != nil {
		return nil, err
	}
	type wire GeneratedScenario
	raw, err := json.Marshal(wire(artifact))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxGeneratedArtifactBytes {
		return nil, fmt.Errorf("%w: public artifact exceeds %d bytes", ErrGeneration, MaxGeneratedArtifactBytes)
	}
	return raw, nil
}

func (artifact GeneratedScenario) clone() GeneratedScenario {
	artifact.World.Descriptor = artifact.World.Descriptor.clone()
	artifact.World.Catalog = artifact.World.Catalog.Clone()
	artifact.World.Entities = cloneEntities(artifact.World.Entities)
	artifact.World.PredicateSchemas = clonePredicateSchemas(artifact.World.PredicateSchemas)
	artifact.World.InitialFacts = clonePredicates(artifact.World.InitialFacts)
	return artifact
}

func textSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
