package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (s *Store) AppendCanonEvent(
	ctx context.Context,
	worldID string,
	sourceMessageID int64,
	content string,
) (CanonEvent, error) {
	if err := s.validateContext(ctx); err != nil {
		return CanonEvent{}, err
	}
	if err := validateIdentity(worldID, worldIdentity); err != nil {
		return CanonEvent{}, err
	}
	if sourceMessageID < 1 {
		return CanonEvent{}, fmt.Errorf("roleplay canon event requires a source message identity")
	}
	if err := validateEventContent(content); err != nil {
		return CanonEvent{}, err
	}
	id, err := newIdentity("rpe_")
	if err != nil {
		return CanonEvent{}, err
	}
	return scanCanonEvent(s.pool.QueryRow(ctx, `
		INSERT INTO roleplay_canon_events (id,world_id,source_message_id,content)
		VALUES ($1,$2,$3,$4)
		RETURNING id,world_id,source_message_id,ordinal,content,authority_namespace,created_at
	`, id, worldID, sourceMessageID, content))
}

func (s *Store) GrantKnowledge(
	ctx context.Context,
	characterID, eventID string,
) (KnowledgeGrant, bool, error) {
	if err := s.validateContext(ctx); err != nil {
		return KnowledgeGrant{}, false, err
	}
	if err := validateIdentity(characterID, characterIdentity); err != nil {
		return KnowledgeGrant{}, false, err
	}
	if err := validateIdentity(eventID, eventIdentity); err != nil {
		return KnowledgeGrant{}, false, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return KnowledgeGrant{}, false, err
	}
	defer tx.Rollback(context.Background())

	characterWorld, err := roleplayObjectWorld(ctx, tx, "roleplay_characters", characterID)
	if err != nil {
		return KnowledgeGrant{}, false, fmt.Errorf("resolve roleplay character world: %w", err)
	}
	eventWorld, err := roleplayObjectWorld(ctx, tx, "roleplay_canon_events", eventID)
	if err != nil {
		return KnowledgeGrant{}, false, fmt.Errorf("resolve roleplay canon event world: %w", err)
	}
	if characterWorld != eventWorld {
		return KnowledgeGrant{}, false, fmt.Errorf(
			"roleplay character and canon event belong to different fictional worlds",
		)
	}
	id, err := newIdentity("rpk_")
	if err != nil {
		return KnowledgeGrant{}, false, err
	}
	grant, err := scanKnowledgeGrant(tx.QueryRow(ctx, `
		INSERT INTO roleplay_character_knowledge (
			id,world_id,character_id,canon_event_id
		) VALUES ($1,$2,$3,$4)
		ON CONFLICT (character_id,canon_event_id) DO NOTHING
		RETURNING id,world_id,character_id,canon_event_id,authority_namespace,created_at
	`, id, characterWorld, characterID, eventID))
	created := true
	if err == pgx.ErrNoRows {
		created = false
		grant, err = scanKnowledgeGrant(tx.QueryRow(ctx, `
			SELECT id,world_id,character_id,canon_event_id,authority_namespace,created_at
			FROM roleplay_character_knowledge
			WHERE character_id=$1 AND canon_event_id=$2
		`, characterID, eventID))
	}
	if err != nil {
		return KnowledgeGrant{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return KnowledgeGrant{}, false, err
	}
	return grant, created, nil
}

func roleplayObjectWorld(ctx context.Context, tx pgx.Tx, table, id string) (string, error) {
	var query string
	switch table {
	case "roleplay_characters":
		query = `SELECT world_id FROM roleplay_characters WHERE id=$1`
	case "roleplay_canon_events":
		query = `SELECT world_id FROM roleplay_canon_events WHERE id=$1`
	default:
		return "", fmt.Errorf("unsupported roleplay world authority %q", table)
	}
	var worldID string
	if err := tx.QueryRow(ctx, query, id).Scan(&worldID); err != nil {
		return "", err
	}
	return worldID, nil
}

func scanCanonEvent(row rowScanner) (CanonEvent, error) {
	var event CanonEvent
	var authority string
	if err := row.Scan(
		&event.ID, &event.WorldID, &event.SourceMessageID, &event.Ordinal,
		&event.Content, &authority, &event.CreatedAt,
	); err != nil {
		return CanonEvent{}, err
	}
	event.Authority = AuthorityNamespace(authority)
	if event.Authority != AuthorityFictionalCanon {
		return CanonEvent{}, fmt.Errorf("roleplay canon event has invalid authority %q", authority)
	}
	return event, nil
}

func scanKnowledgeGrant(row rowScanner) (KnowledgeGrant, error) {
	var grant KnowledgeGrant
	var authority string
	if err := row.Scan(
		&grant.ID, &grant.WorldID, &grant.CharacterID, &grant.EventID,
		&authority, &grant.CreatedAt,
	); err != nil {
		return KnowledgeGrant{}, err
	}
	grant.Authority = AuthorityNamespace(authority)
	if grant.Authority != AuthorityCharacterKnowledge {
		return KnowledgeGrant{}, fmt.Errorf("roleplay knowledge has invalid authority %q", authority)
	}
	return grant, nil
}
