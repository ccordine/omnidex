package roleplay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

const MaxCharacterLibraryPageSize = 50

type LibraryCharacterSummary struct {
	ID                    string             `json:"id"`
	Name                  string             `json:"name"`
	Authority             AuthorityNamespace `json:"authority"`
	Profile               *PersonaSheet      `json:"profile,omitempty"`
	ProfileRevision       int64              `json:"profile_revision,omitempty"`
	MemoryCount           int64              `json:"memory_count"`
	PlacementCount        int64              `json:"placement_count"`
	PlacedInSelectedWorld bool               `json:"placed_in_selected_world"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`
}

type LibraryCharacterPage struct {
	Items           []LibraryCharacterSummary `json:"items"`
	SelectedWorldID string                    `json:"selected_world_id,omitempty"`
	HasMore         bool                      `json:"has_more"`
	Offset          int                       `json:"offset"`
}

type WorldSummary struct {
	World          World  `json:"world"`
	SceneTitle     string `json:"scene_title,omitempty"`
	CharacterCount int64  `json:"character_count"`
}

type WorldPage struct {
	Items   []WorldSummary `json:"items"`
	HasMore bool           `json:"has_more"`
	Offset  int            `json:"offset"`
}

func (s *Store) CreateLibraryCharacter(ctx context.Context, name string) (LibraryCharacterSummary, error) {
	if err := s.validateContext(ctx); err != nil {
		return LibraryCharacterSummary{}, err
	}
	if err := validateName(name, "roleplay library character name"); err != nil {
		return LibraryCharacterSummary{}, err
	}
	id, err := NewLibraryCharacterIdentity()
	if err != nil {
		return LibraryCharacterSummary{}, err
	}
	return scanLibraryCharacter(s.pool.QueryRow(ctx, `
		INSERT INTO roleplay_character_library (id,name)
		VALUES ($1,$2)
		RETURNING id,name,authority_namespace,NULL::bigint,NULL::text,NULL::text,
		          NULL::jsonb,NULL::jsonb,0::bigint,0::bigint,FALSE,created_at,updated_at
	`, id, name))
}

func (s *Store) PlaceLibraryCharacter(ctx context.Context, worldID, libraryID string) (Character, error) {
	if err := s.validateContext(ctx); err != nil {
		return Character{}, err
	}
	if err := validateIdentity(worldID, worldIdentity); err != nil {
		return Character{}, err
	}
	if err := validateIdentity(libraryID, libraryCharacterIdentity); err != nil {
		return Character{}, err
	}
	id, err := NewCharacterIdentity()
	if err != nil {
		return Character{}, err
	}
	character, err := scanCharacter(s.pool.QueryRow(ctx, `
		INSERT INTO roleplay_characters (id,world_id,library_character_id,name)
		SELECT $1,$2,library.id,library.name
		FROM roleplay_character_library AS library
		WHERE library.id=$3
		RETURNING id,world_id,library_character_id,name,authority_namespace,created_at
	`, id, worldID, libraryID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Character{}, fmt.Errorf("%w: library character is absent", ErrSimulationNotConfigured)
	}
	if err != nil {
		return Character{}, simulationDefinitionError("character placement", err)
	}
	return character, nil
}

func (s *Store) ListLibraryCharactersPage(
	ctx context.Context,
	selectedWorldID string,
	limit, offset int,
) (LibraryCharacterPage, error) {
	if err := s.validateContext(ctx); err != nil {
		return LibraryCharacterPage{}, err
	}
	if err := validateLibraryPage(limit, offset); err != nil {
		return LibraryCharacterPage{}, err
	}
	if selectedWorldID != "" {
		if err := validateIdentity(selectedWorldID, worldIdentity); err != nil {
			return LibraryCharacterPage{}, err
		}
	}
	rows, err := s.pool.Query(ctx, `
		SELECT library.id,library.name,library.authority_namespace,
		       profile.revision,profile.summary,profile.voice,profile.traits,profile.goals,
		       COUNT(DISTINCT memory.id),COUNT(DISTINCT placement.id),
		       COUNT(DISTINCT selected_placement.id)=1,
		       library.created_at,library.updated_at
		FROM roleplay_character_library AS library
		LEFT JOIN roleplay_character_profiles AS profile
		  ON profile.library_character_id=library.id
		LEFT JOIN roleplay_characters AS placement
		  ON placement.library_character_id=library.id
		LEFT JOIN roleplay_character_memories AS memory
		  ON memory.character_id=placement.id
		LEFT JOIN roleplay_characters AS selected_placement
		  ON selected_placement.library_character_id=library.id
		 AND selected_placement.world_id=NULLIF($3,'')
		GROUP BY library.id,profile.library_character_id
		ORDER BY library.created_at ASC,library.id ASC
		LIMIT $1 OFFSET $2
	`, limit+1, offset, selectedWorldID)
	if err != nil {
		return LibraryCharacterPage{}, err
	}
	defer rows.Close()
	items := make([]LibraryCharacterSummary, 0, limit+1)
	for rows.Next() {
		item, scanErr := scanLibraryCharacter(rows)
		if scanErr != nil {
			return LibraryCharacterPage{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return LibraryCharacterPage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return LibraryCharacterPage{
		Items: items, SelectedWorldID: selectedWorldID, HasMore: hasMore, Offset: offset,
	}, nil
}

func (s *Store) ListWorldsPage(ctx context.Context, limit, offset int) (WorldPage, error) {
	if err := s.validateContext(ctx); err != nil {
		return WorldPage{}, err
	}
	if err := validateLibraryPage(limit, offset); err != nil {
		return WorldPage{}, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT world.id,world.channel_id,world.name,world.authority_namespace,world.created_at,
		       COALESCE(scene.title,''),COUNT(character.id)
		FROM roleplay_worlds AS world
		LEFT JOIN roleplay_current_scenes AS scene ON scene.world_id=world.id
		LEFT JOIN roleplay_characters AS character ON character.world_id=world.id
		GROUP BY world.id,scene.id
		ORDER BY world.created_at DESC,world.id ASC
		LIMIT $1 OFFSET $2
	`, limit+1, offset)
	if err != nil {
		return WorldPage{}, err
	}
	defer rows.Close()
	items := make([]WorldSummary, 0, limit+1)
	for rows.Next() {
		var item WorldSummary
		var authority string
		if err := rows.Scan(
			&item.World.ID, &item.World.ChannelID, &item.World.Name, &authority,
			&item.World.CreatedAt, &item.SceneTitle, &item.CharacterCount,
		); err != nil {
			return WorldPage{}, err
		}
		item.World.Authority = AuthorityNamespace(authority)
		if item.World.Authority != AuthorityFictionalCanon {
			return WorldPage{}, fmt.Errorf("roleplay world has invalid authority %q", authority)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return WorldPage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return WorldPage{Items: items, HasMore: hasMore, Offset: offset}, nil
}

func validateLibraryPage(limit, offset int) error {
	if limit < 1 || limit > MaxCharacterLibraryPageSize || offset < 0 {
		return fmt.Errorf("roleplay library page requires limit 1..%d and nonnegative offset", MaxCharacterLibraryPageSize)
	}
	return nil
}

func scanLibraryCharacter(row rowScanner) (LibraryCharacterSummary, error) {
	var item LibraryCharacterSummary
	var authority string
	var revision *int64
	var summary, voice *string
	var traits, goals []byte
	if err := row.Scan(
		&item.ID, &item.Name, &authority, &revision, &summary, &voice, &traits, &goals,
		&item.MemoryCount, &item.PlacementCount, &item.PlacedInSelectedWorld,
		&item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return LibraryCharacterSummary{}, err
	}
	item.Authority = AuthorityNamespace(authority)
	if item.Authority != AuthorityCharacterIdentity {
		return LibraryCharacterSummary{}, fmt.Errorf("library character has invalid authority %q", authority)
	}
	if err := validateIdentity(item.ID, libraryCharacterIdentity); err != nil {
		return LibraryCharacterSummary{}, err
	}
	if revision == nil {
		if summary != nil || voice != nil || traits != nil || goals != nil {
			return LibraryCharacterSummary{}, fmt.Errorf("library character has a partial profile")
		}
		return item, nil
	}
	if summary == nil || voice == nil || traits == nil || goals == nil {
		return LibraryCharacterSummary{}, fmt.Errorf("library character has a partial profile")
	}
	profile := PersonaSheet{Summary: *summary, Voice: *voice}
	if err := json.Unmarshal(traits, &profile.Traits); err != nil {
		return LibraryCharacterSummary{}, fmt.Errorf("decode library character traits: %w", err)
	}
	if err := json.Unmarshal(goals, &profile.Goals); err != nil {
		return LibraryCharacterSummary{}, fmt.Errorf("decode library character goals: %w", err)
	}
	if err := validatePersonaSheet(profile); err != nil {
		return LibraryCharacterSummary{}, fmt.Errorf("persisted library character profile is invalid: %w", err)
	}
	item.Profile = &profile
	item.ProfileRevision = *revision
	return item, nil
}
