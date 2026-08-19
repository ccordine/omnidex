package roleplay

import (
	"context"
	"fmt"
)

const MaxSimulationPageSize = 50

type SimulationCharacterPage struct {
	Items   []SimulationCharacterSummary `json:"items"`
	HasMore bool                         `json:"has_more"`
}

type PersonaPage struct {
	Items   []PersonaProjection `json:"items"`
	HasMore bool                `json:"has_more"`
}

func (s *Store) ListSimulationCharactersPage(
	ctx context.Context,
	worldID string,
	limit, offset int,
) (SimulationCharacterPage, error) {
	if err := s.validateContext(ctx); err != nil {
		return SimulationCharacterPage{}, err
	}
	if err := validateSimulationPage(worldID, limit, offset); err != nil {
		return SimulationCharacterPage{}, err
	}
	return loadSimulationCharactersPage(ctx, s.pool, worldID, limit, offset)
}

func loadSimulationCharactersPage(
	ctx context.Context,
	query simulationQuerier,
	worldID string,
	limit, offset int,
) (SimulationCharacterPage, error) {
	rows, err := query.Query(ctx, `
		SELECT id,world_id,name,authority_namespace,created_at
		FROM roleplay_characters
		WHERE world_id=$1
		ORDER BY created_at ASC,id ASC
		LIMIT $2 OFFSET $3
	`, worldID, limit+1, offset)
	if err != nil {
		return SimulationCharacterPage{}, err
	}
	defer rows.Close()
	items := make([]SimulationCharacterSummary, 0, limit+1)
	for rows.Next() {
		var item SimulationCharacterSummary
		var authority AuthorityNamespace
		if err := rows.Scan(&item.ID, &item.WorldID, &item.Name, &authority, &item.CreatedAt); err != nil {
			return SimulationCharacterPage{}, err
		}
		if authority != AuthorityFictionalCanon {
			return SimulationCharacterPage{}, fmt.Errorf("persisted character has invalid authority %q", authority)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return SimulationCharacterPage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return SimulationCharacterPage{Items: items, HasMore: hasMore}, nil
}

func (s *Store) ListPersonaPage(
	ctx context.Context,
	worldID string,
	limit, offset int,
) (PersonaPage, error) {
	if err := s.validateContext(ctx); err != nil {
		return PersonaPage{}, err
	}
	if err := validateSimulationPage(worldID, limit, offset); err != nil {
		return PersonaPage{}, err
	}
	return loadPersonaPage(ctx, s.pool, worldID, limit, offset)
}

func loadPersonaPage(
	ctx context.Context,
	query simulationQuerier,
	worldID string,
	limit, offset int,
) (PersonaPage, error) {
	rows, err := query.Query(ctx, `
		SELECT persona.character_id,persona.revision,persona.summary,persona.voice,
		       persona.traits,persona.goals,persona.updated_at
		FROM roleplay_character_personas AS persona
		WHERE persona.world_id=$1
		ORDER BY persona.character_id ASC
		LIMIT $2 OFFSET $3
	`, worldID, limit+1, offset)
	if err != nil {
		return PersonaPage{}, err
	}
	defer rows.Close()
	items := make([]PersonaProjection, 0, limit+1)
	for rows.Next() {
		projection, err := scanPersonaProjection(rows)
		if err != nil {
			return PersonaPage{}, err
		}
		items = append(items, projection)
	}
	if err := rows.Err(); err != nil {
		return PersonaPage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return PersonaPage{Items: items, HasMore: hasMore}, nil
}

func validateSimulationPage(worldID string, limit, offset int) error {
	if err := validateIdentity(worldID, worldIdentity); err != nil {
		return err
	}
	if limit < 1 || limit > MaxSimulationPageSize || offset < 0 {
		return fmt.Errorf("simulation page requires limit 1..%d and nonnegative offset", MaxSimulationPageSize)
	}
	return nil
}
