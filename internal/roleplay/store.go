package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) (*Store, error) {
	if pool == nil {
		return nil, fmt.Errorf("roleplay store requires PostgreSQL")
	}
	return &Store{pool: pool}, nil
}

func (s *Store) FindWorldByChannel(ctx context.Context, channelID string) (World, bool, error) {
	if err := s.validateContext(ctx); err != nil {
		return World{}, false, err
	}
	if err := validateChannelID(channelID); err != nil {
		return World{}, false, err
	}
	world, err := scanWorld(s.pool.QueryRow(ctx, `
		SELECT id,channel_id,name,authority_namespace,created_at
		FROM roleplay_worlds
		WHERE channel_id=$1
	`, channelID))
	if err == pgx.ErrNoRows {
		return World{}, false, nil
	}
	if err != nil {
		return World{}, false, err
	}
	return world, true, nil
}

func BootstrapWorldTx(
	ctx context.Context,
	tx pgx.Tx,
	channelID, worldID, worldName, viewpointID, viewpointName string,
) (World, Character, error) {
	if ctx == nil || tx == nil {
		return World{}, Character{}, fmt.Errorf("roleplay world bootstrap requires transaction authority")
	}
	if err := validateChannelID(channelID); err != nil {
		return World{}, Character{}, err
	}
	if err := validateIdentity(worldID, worldIdentity); err != nil {
		return World{}, Character{}, err
	}
	if err := validateIdentity(viewpointID, characterIdentity); err != nil {
		return World{}, Character{}, err
	}
	if err := validateName(worldName, "roleplay world name"); err != nil {
		return World{}, Character{}, err
	}
	if err := validateName(viewpointName, "roleplay character name"); err != nil {
		return World{}, Character{}, err
	}
	libraryID, err := NewLibraryCharacterIdentity()
	if err != nil {
		return World{}, Character{}, err
	}
	world, err := scanWorld(tx.QueryRow(ctx, `
		INSERT INTO roleplay_worlds (id,channel_id,name)
		VALUES ($1,$2,$3)
		RETURNING id,channel_id,name,authority_namespace,created_at
	`, worldID, channelID, worldName))
	if err != nil {
		return World{}, Character{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO roleplay_character_library (id,name)
		VALUES ($1,$2)
	`, libraryID, viewpointName); err != nil {
		return World{}, Character{}, err
	}
	character, err := scanCharacter(tx.QueryRow(ctx, `
		INSERT INTO roleplay_characters (id,world_id,library_character_id,name)
		VALUES ($1,$2,$3,$4)
		RETURNING id,world_id,library_character_id,name,authority_namespace,created_at
	`, viewpointID, world.ID, libraryID, viewpointName))
	if err != nil {
		return World{}, Character{}, err
	}
	return world, character, nil
}

func (s *Store) CreateCharacter(ctx context.Context, worldID, name string) (Character, error) {
	if err := s.validateContext(ctx); err != nil {
		return Character{}, err
	}
	if err := validateIdentity(worldID, worldIdentity); err != nil {
		return Character{}, err
	}
	if err := validateName(name, "roleplay character name"); err != nil {
		return Character{}, err
	}
	id, err := newIdentity("rpc_")
	if err != nil {
		return Character{}, err
	}
	libraryID, err := NewLibraryCharacterIdentity()
	if err != nil {
		return Character{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Character{}, err
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `
		INSERT INTO roleplay_character_library (id,name)
		VALUES ($1,$2)
	`, libraryID, name); err != nil {
		return Character{}, err
	}
	character, err := scanCharacter(tx.QueryRow(ctx, `
		INSERT INTO roleplay_characters (id,world_id,library_character_id,name)
		VALUES ($1,$2,$3,$4)
		RETURNING id,world_id,library_character_id,name,authority_namespace,created_at
	`, id, worldID, libraryID, name))
	if err != nil {
		return Character{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Character{}, err
	}
	return character, nil
}

func (s *Store) ListCharacters(ctx context.Context, worldID string) ([]Character, error) {
	if err := s.validateContext(ctx); err != nil {
		return nil, err
	}
	if err := validateIdentity(worldID, worldIdentity); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id,world_id,library_character_id,name,authority_namespace,created_at
		FROM roleplay_characters
		WHERE world_id=$1
		ORDER BY created_at ASC,id ASC
	`, worldID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	characters := make([]Character, 0)
	for rows.Next() {
		character, err := scanCharacter(rows)
		if err != nil {
			return nil, err
		}
		characters = append(characters, character)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return characters, nil
}

func (s *Store) validateContext(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("roleplay store requires PostgreSQL")
	}
	if ctx == nil {
		return fmt.Errorf("roleplay store operation requires context")
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanWorld(row rowScanner) (World, error) {
	var world World
	var authority string
	if err := row.Scan(
		&world.ID, &world.ChannelID, &world.Name, &authority, &world.CreatedAt,
	); err != nil {
		return World{}, err
	}
	world.Authority = AuthorityNamespace(authority)
	if world.Authority != AuthorityFictionalCanon {
		return World{}, fmt.Errorf("roleplay world has invalid authority %q", authority)
	}
	return world, nil
}

func scanCharacter(row rowScanner) (Character, error) {
	var character Character
	var authority string
	if err := row.Scan(
		&character.ID, &character.WorldID, &character.LibraryID,
		&character.Name, &authority, &character.CreatedAt,
	); err != nil {
		return Character{}, err
	}
	character.Authority = AuthorityNamespace(authority)
	if character.Authority != AuthorityFictionalCanon {
		return Character{}, fmt.Errorf("roleplay character has invalid authority %q", authority)
	}
	if err := validateIdentity(character.LibraryID, libraryCharacterIdentity); err != nil {
		return Character{}, fmt.Errorf("roleplay character has invalid library authority: %w", err)
	}
	return character, nil
}
