package roleplay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

func newCharacterProjection(
	world World,
	character Character,
	events []projectedEvent,
) (CharacterProjection, error) {
	if err := validateProjectionAuthority(world, character); err != nil {
		return CharacterProjection{}, err
	}
	projection := CharacterProjection{
		Schema: CharacterProjectionSchemaV1, Authority: AuthorityCharacterKnowledge,
		WorldID: world.ID, WorldName: world.Name,
		CharacterID: character.ID, CharacterName: character.Name,
		Facts: make([]ContextFact, 0, len(events)),
	}
	for _, event := range events {
		if err := validateIdentity(event.ID, eventIdentity); err != nil {
			return CharacterProjection{}, err
		}
		if err := validateEventContent(event.content); err != nil {
			return CharacterProjection{}, err
		}
		projection.Facts = append(projection.Facts, ContextFact{EventID: event.ID, Content: event.content})
	}
	fingerprint, err := ExactCharacterProjectionFingerprint(projection)
	if err != nil {
		return CharacterProjection{}, err
	}
	projection.Fingerprint = fingerprint
	return projection, nil
}

func (projection CharacterProjection) Validate() error {
	if projection.Schema != CharacterProjectionSchemaV1 || projection.Authority != AuthorityCharacterKnowledge {
		return fmt.Errorf("roleplay character projection has invalid schema or authority")
	}
	if len(projection.Facts) > MaxProjectionEvents {
		return fmt.Errorf("roleplay character projection exceeds the event bound")
	}
	if err := validateProjectionAuthority(
		World{ID: projection.WorldID, Name: projection.WorldName},
		Character{ID: projection.CharacterID, WorldID: projection.WorldID, Name: projection.CharacterName},
	); err != nil {
		return err
	}
	if err := validateProjectionFacts(projection.Facts, "character projection"); err != nil {
		return err
	}
	expected, err := ExactCharacterProjectionFingerprint(projection)
	if err != nil {
		return err
	}
	if projection.Fingerprint != expected {
		return fmt.Errorf("roleplay character projection fingerprint does not match exact authority")
	}
	return nil
}

func ExactCharacterProjectionFingerprint(projection CharacterProjection) (string, error) {
	projection.Fingerprint = ""
	return projectionFingerprint(projection)
}

func newCanonProjection(world World, events []projectedEvent) (CanonProjection, error) {
	if err := validateIdentity(world.ID, worldIdentity); err != nil {
		return CanonProjection{}, err
	}
	if err := validateName(world.Name, "roleplay world name"); err != nil {
		return CanonProjection{}, err
	}
	projection := CanonProjection{
		Schema: CanonProjectionSchemaV1, Authority: AuthorityFictionalCanon,
		WorldID: world.ID, WorldName: world.Name, Facts: make([]ContextFact, 0, len(events)),
	}
	for _, event := range events {
		if err := validateIdentity(event.ID, eventIdentity); err != nil {
			return CanonProjection{}, err
		}
		if err := validateEventContent(event.content); err != nil {
			return CanonProjection{}, err
		}
		projection.Facts = append(projection.Facts, ContextFact{EventID: event.ID, Content: event.content})
	}
	fingerprint, err := ExactCanonProjectionFingerprint(projection)
	if err != nil {
		return CanonProjection{}, err
	}
	projection.Fingerprint = fingerprint
	return projection, nil
}

func (projection CanonProjection) Validate() error {
	if projection.Schema != CanonProjectionSchemaV1 || projection.Authority != AuthorityFictionalCanon {
		return fmt.Errorf("roleplay canon projection has invalid schema or authority")
	}
	if len(projection.Facts) > MaxProjectionEvents {
		return fmt.Errorf("roleplay canon projection exceeds the event bound")
	}
	if err := validateIdentity(projection.WorldID, worldIdentity); err != nil {
		return err
	}
	if err := validateName(projection.WorldName, "roleplay world name"); err != nil {
		return err
	}
	if err := validateProjectionFacts(projection.Facts, "canon projection"); err != nil {
		return err
	}
	expected, err := ExactCanonProjectionFingerprint(projection)
	if err != nil {
		return err
	}
	if projection.Fingerprint != expected {
		return fmt.Errorf("roleplay canon projection fingerprint does not match exact authority")
	}
	return nil
}

func validateProjectionFacts(facts []ContextFact, label string) error {
	total := 0
	seen := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		if err := validateIdentity(fact.EventID, eventIdentity); err != nil {
			return err
		}
		if _, duplicate := seen[fact.EventID]; duplicate {
			return fmt.Errorf("roleplay %s repeats event %q", label, fact.EventID)
		}
		seen[fact.EventID] = struct{}{}
		if err := validateEventContent(fact.Content); err != nil {
			return err
		}
		total += len(fact.Content)
	}
	if total > MaxProjectionContentBytes {
		return fmt.Errorf("roleplay %s exceeds %d content bytes", label, MaxProjectionContentBytes)
	}
	return nil
}

func ExactCanonProjectionFingerprint(projection CanonProjection) (string, error) {
	projection.Fingerprint = ""
	return projectionFingerprint(projection)
}

func validateProjectionAuthority(world World, character Character) error {
	if err := validateIdentity(world.ID, worldIdentity); err != nil {
		return err
	}
	if err := validateIdentity(character.ID, characterIdentity); err != nil {
		return err
	}
	if character.WorldID != "" && character.WorldID != world.ID {
		return fmt.Errorf("roleplay character does not belong to projection world")
	}
	if err := validateName(world.Name, "roleplay world name"); err != nil {
		return err
	}
	return validateName(character.Name, "roleplay character name")
}

func projectionFingerprint(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}
