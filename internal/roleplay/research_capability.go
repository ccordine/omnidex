package roleplay

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
)

type CharacterCapability string

const (
	CapabilityWebResearch CharacterCapability = "web_research"
	AuthorityRealWorld    AuthorityNamespace  = "REAL_WORLD"
	AuthorityCodeIssued   AuthorityNamespace  = "CODE_ISSUED_CAPABILITY"
)

var (
	ErrResearchCapabilityDenied = errors.New("roleplay character lacks web_research capability")
	capabilityGrantIdentity     = identityKind{
		name:    "roleplay capability grant",
		pattern: regexp.MustCompile(`^rpg_[0-9a-f]{32}$`),
	}
)

type CharacterCapabilityProjection struct {
	WorldID           string    `json:"world_id"`
	CharacterID       string    `json:"character_id"`
	WebResearch       bool      `json:"web_research"`
	WebResearchGrant  string    `json:"web_research_grant_id,omitempty"`
	WebResearchIssued time.Time `json:"web_research_issued_at,omitempty"`
}

func (s *Store) ConfigureCharacterCapability(
	ctx context.Context,
	worldID, characterID string,
	capability CharacterCapability,
	enabled bool,
) (CharacterCapabilityProjection, error) {
	if err := s.validateContext(ctx); err != nil {
		return CharacterCapabilityProjection{}, err
	}
	if err := validateResearchCapabilityIdentity(worldID, characterID, capability); err != nil {
		return CharacterCapabilityProjection{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CharacterCapabilityProjection{}, err
	}
	defer tx.Rollback(context.Background())
	var lockedCharacterID string
	if err := tx.QueryRow(ctx, `
		SELECT id FROM roleplay_characters
		WHERE world_id=$1 AND id=$2
		FOR UPDATE
	`, worldID, characterID).Scan(&lockedCharacterID); err == pgx.ErrNoRows {
		return CharacterCapabilityProjection{}, fmt.Errorf("roleplay capability character does not belong to world")
	} else if err != nil {
		return CharacterCapabilityProjection{}, err
	}
	if enabled {
		var currentGrantID *string
		if err := tx.QueryRow(ctx, `
			SELECT grant_id FROM roleplay_character_capabilities
			WHERE world_id=$1 AND character_id=$2 AND capability=$3
		`, worldID, characterID, capability).Scan(&currentGrantID); err != nil && err != pgx.ErrNoRows {
			return CharacterCapabilityProjection{}, err
		}
		if currentGrantID == nil {
			grantID, err := newIdentity("rpg_")
			if err != nil {
				return CharacterCapabilityProjection{}, err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO roleplay_character_capability_grants (
					grant_id,world_id,character_id,capability
				) VALUES ($1,$2,$3,$4)
			`, grantID, worldID, characterID, capability); err != nil {
				return CharacterCapabilityProjection{}, fmt.Errorf("issue roleplay character capability: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO roleplay_character_capabilities (
					grant_id,world_id,character_id,capability
				) VALUES ($1,$2,$3,$4)
			`, grantID, worldID, characterID, capability); err != nil {
				return CharacterCapabilityProjection{}, fmt.Errorf("enable roleplay character capability: %w", err)
			}
		}
	} else if _, err := tx.Exec(ctx, `
		DELETE FROM roleplay_character_capabilities
		WHERE world_id=$1 AND character_id=$2 AND capability=$3
	`, worldID, characterID, capability); err != nil {
		return CharacterCapabilityProjection{}, fmt.Errorf("disable roleplay character capability: %w", err)
	}
	projection, err := projectCharacterCapabilityTx(ctx, tx, worldID, characterID)
	if err != nil {
		return CharacterCapabilityProjection{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CharacterCapabilityProjection{}, err
	}
	return projection, nil
}

func (s *Store) ProjectCharacterCapability(
	ctx context.Context,
	worldID, characterID string,
) (CharacterCapabilityProjection, error) {
	if err := s.validateContext(ctx); err != nil {
		return CharacterCapabilityProjection{}, err
	}
	if err := validateResearchCapabilityIdentity(worldID, characterID, CapabilityWebResearch); err != nil {
		return CharacterCapabilityProjection{}, err
	}
	return projectCharacterCapabilityTx(ctx, s.pool, worldID, characterID)
}

func projectCharacterCapabilityTx(
	ctx context.Context,
	querier interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	worldID, characterID string,
) (CharacterCapabilityProjection, error) {
	projection := CharacterCapabilityProjection{WorldID: worldID, CharacterID: characterID}
	var grantID *string
	var issuedAt *time.Time
	err := querier.QueryRow(ctx, `
		SELECT capability.grant_id,capability_grant.created_at
		FROM roleplay_characters AS character
		LEFT JOIN roleplay_character_capabilities AS capability
		  ON capability.world_id=character.world_id
		 AND capability.character_id=character.id
		 AND capability.capability='web_research'
		LEFT JOIN roleplay_character_capability_grants AS capability_grant
		  ON capability_grant.grant_id=capability.grant_id
		 AND capability_grant.world_id=capability.world_id
		 AND capability_grant.character_id=capability.character_id
		 AND capability_grant.capability=capability.capability
		WHERE character.world_id=$1 AND character.id=$2
	`, worldID, characterID).Scan(&grantID, &issuedAt)
	if err == pgx.ErrNoRows {
		return CharacterCapabilityProjection{}, fmt.Errorf("roleplay capability character does not belong to world")
	}
	if err != nil {
		return CharacterCapabilityProjection{}, err
	}
	if grantID != nil || issuedAt != nil {
		if grantID == nil || issuedAt == nil || validateIdentity(*grantID, capabilityGrantIdentity) != nil {
			return CharacterCapabilityProjection{}, fmt.Errorf("persisted roleplay research capability is invalid")
		}
		projection.WebResearch = true
		projection.WebResearchGrant = *grantID
		projection.WebResearchIssued = issuedAt.UTC()
	}
	return projection, nil
}

func validateResearchCapabilityIdentity(
	worldID, characterID string,
	capability CharacterCapability,
) error {
	if err := validateIdentity(worldID, worldIdentity); err != nil {
		return err
	}
	if err := validateIdentity(characterID, characterIdentity); err != nil {
		return err
	}
	if capability != CapabilityWebResearch {
		return fmt.Errorf("roleplay character capability %q is unsupported", capability)
	}
	return nil
}
