package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Store) ProjectCharacterContext(
	ctx context.Context,
	characterID string,
	limit int,
) (CharacterProjection, error) {
	return s.projectCharacterContext(ctx, "", characterID, limit)
}

func (s *Store) ProjectChannelCharacterContext(
	ctx context.Context,
	channelID string,
	characterID string,
	limit int,
) (CharacterProjection, error) {
	if err := validateChannelID(channelID); err != nil {
		return CharacterProjection{}, err
	}
	return s.projectCharacterContext(ctx, channelID, characterID, limit)
}

func (s *Store) projectCharacterContext(
	ctx context.Context,
	channelID string,
	characterID string,
	limit int,
) (CharacterProjection, error) {
	if err := s.validateContext(ctx); err != nil {
		return CharacterProjection{}, err
	}
	if err := validateIdentity(characterID, characterIdentity); err != nil {
		return CharacterProjection{}, err
	}
	if err := validateProjectionLimit(limit); err != nil {
		return CharacterProjection{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return CharacterProjection{}, err
	}
	defer tx.Rollback(context.Background())
	world, character, err := characterAuthority(ctx, tx, channelID, characterID)
	if err != nil {
		return CharacterProjection{}, err
	}
	events, err := characterEvents(ctx, tx, character.ID, limit)
	if err != nil {
		return CharacterProjection{}, err
	}
	projection, err := newCharacterProjection(world, character, events)
	if err != nil {
		return CharacterProjection{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CharacterProjection{}, err
	}
	return projection, nil
}

func (s *Store) ProjectCanonContext(
	ctx context.Context,
	worldID string,
	limit int,
) (CanonProjection, error) {
	return s.projectCanonContext(ctx, worldID, "", limit)
}

func (s *Store) ProjectChannelCanonContext(
	ctx context.Context,
	channelID string,
	limit int,
) (CanonProjection, error) {
	if err := validateChannelID(channelID); err != nil {
		return CanonProjection{}, err
	}
	return s.projectCanonContext(ctx, "", channelID, limit)
}

func (s *Store) projectCanonContext(
	ctx context.Context,
	worldID string,
	channelID string,
	limit int,
) (CanonProjection, error) {
	if err := s.validateContext(ctx); err != nil {
		return CanonProjection{}, err
	}
	if channelID == "" {
		if err := validateIdentity(worldID, worldIdentity); err != nil {
			return CanonProjection{}, err
		}
	} else if worldID != "" {
		return CanonProjection{}, fmt.Errorf("roleplay canon projection has conflicting world/channel authority")
	}
	if err := validateProjectionLimit(limit); err != nil {
		return CanonProjection{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return CanonProjection{}, err
	}
	defer tx.Rollback(context.Background())
	var world World
	if channelID == "" {
		world, err = worldAuthority(ctx, tx, worldID)
	} else {
		world, err = worldAuthorityForChannel(ctx, tx, channelID)
	}
	if err != nil {
		return CanonProjection{}, err
	}
	events, err := canonEvents(ctx, tx, world.ID, limit)
	if err != nil {
		return CanonProjection{}, err
	}
	projection, err := newCanonProjection(world, events)
	if err != nil {
		return CanonProjection{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CanonProjection{}, err
	}
	return projection, nil
}

func characterAuthority(
	ctx context.Context,
	tx pgx.Tx,
	channelID string,
	characterID string,
) (World, Character, error) {
	var world World
	var character Character
	var worldAuthority, characterAuthority string
	err := tx.QueryRow(ctx, `
		SELECT world.id,world.channel_id,world.name,world.authority_namespace,world.created_at,
		       character.id,character.world_id,character.name,
		       character.authority_namespace,character.created_at
		FROM roleplay_characters AS character
		JOIN roleplay_worlds AS world ON world.id=character.world_id
		WHERE character.id=$1 AND ($2='' OR world.channel_id=$2)
	`, characterID, channelID).Scan(
		&world.ID, &world.ChannelID, &world.Name, &worldAuthority, &world.CreatedAt,
		&character.ID, &character.WorldID, &character.Name,
		&characterAuthority, &character.CreatedAt,
	)
	if err != nil {
		return World{}, Character{}, err
	}
	world.Authority = AuthorityNamespace(worldAuthority)
	character.Authority = AuthorityNamespace(characterAuthority)
	if world.Authority != AuthorityFictionalCanon || character.Authority != AuthorityFictionalCanon {
		return World{}, Character{}, fmt.Errorf("roleplay character authority is invalid")
	}
	return world, character, nil
}

func worldAuthority(ctx context.Context, tx pgx.Tx, worldID string) (World, error) {
	return scanWorld(tx.QueryRow(ctx, `
		SELECT id,channel_id,name,authority_namespace,created_at
		FROM roleplay_worlds WHERE id=$1
	`, worldID))
}

func worldAuthorityForChannel(ctx context.Context, tx pgx.Tx, channelID string) (World, error) {
	return scanWorld(tx.QueryRow(ctx, `
		SELECT id,channel_id,name,authority_namespace,created_at
		FROM roleplay_worlds WHERE channel_id=$1
	`, channelID))
}

func characterEvents(
	ctx context.Context,
	tx pgx.Tx,
	characterID string,
	limit int,
) ([]projectedEvent, error) {
	rows, err := tx.Query(ctx, `
		SELECT event.id,event.content
		FROM roleplay_character_knowledge AS knowledge
		JOIN roleplay_canon_events AS event
		  ON event.world_id=knowledge.world_id
		 AND event.id=knowledge.canon_event_id
		WHERE knowledge.character_id=$1
		ORDER BY event.ordinal DESC,event.id DESC
		LIMIT $2
	`, characterID, limit)
	if err != nil {
		return nil, err
	}
	return scanProjectedEvents(rows)
}

func canonEvents(
	ctx context.Context,
	tx pgx.Tx,
	worldID string,
	limit int,
) ([]projectedEvent, error) {
	rows, err := tx.Query(ctx, `
		SELECT id,content
		FROM roleplay_canon_events
		WHERE world_id=$1
		ORDER BY ordinal DESC,id DESC
		LIMIT $2
	`, worldID, limit)
	if err != nil {
		return nil, err
	}
	return scanProjectedEvents(rows)
}

func scanProjectedEvents(rows pgx.Rows) ([]projectedEvent, error) {
	defer rows.Close()
	events := make([]projectedEvent, 0)
	for rows.Next() {
		var event projectedEvent
		if err := rows.Scan(&event.ID, &event.content); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for left, right := 0, len(events)-1; left < right; left, right = left+1, right-1 {
		events[left], events[right] = events[right], events[left]
	}
	return events, nil
}
