package host

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool   *pgxpool.Pool
	schema string
}

func NewStore(pool *pgxpool.Pool) (*Store, error) {
	return NewStoreInSchema(pool, "labyrinth_host")
}

func NewStoreInSchema(pool *pgxpool.Pool, schema string) (*Store, error) {
	if pool == nil {
		return nil, ErrNotConfigured
	}
	if schema == "" || schema != strings.TrimSpace(schema) || len(schema) > 128 ||
		!utf8.ValidString(schema) || strings.ContainsRune(schema, '\x00') {
		return nil, fmt.Errorf("%w: durable host schema is invalid", ErrNotConfigured)
	}
	return &Store{pool: pool, schema: schema}, nil
}

func (store *Store) validate(ctx context.Context) error {
	if ctx == nil || store == nil || store.pool == nil || store.schema == "" {
		return ErrNotConfigured
	}
	return nil
}

func (store *Store) Episode(
	ctx context.Context,
	episode cognition.EpisodeRef,
) (EpisodeReceipt, error) {
	if err := store.validate(ctx); err != nil {
		return EpisodeReceipt{}, err
	}
	if err := episode.Validate(); err != nil {
		return EpisodeReceipt{}, err
	}
	stored, err := loadEpisodeRow(ctx, store.pool, store.schema, episode, false)
	if err != nil {
		return EpisodeReceipt{}, err
	}
	return EpisodeReceipt{
		Episode: stored.Episode, Scenario: stored.Scenario, Start: stored.Start.Clone(),
		Current: stored.Current, Terminal: stored.Terminal,
	}, nil
}

func (store *Store) Action(
	ctx context.Context,
	episode cognition.EpisodeRef,
	actionID cognition.ActionID,
) (ActionReceipt, error) {
	if err := store.validate(ctx); err != nil {
		return ActionReceipt{}, err
	}
	if err := episode.Validate(); err != nil {
		return ActionReceipt{}, err
	}
	if actionID == "" {
		return ActionReceipt{}, fmt.Errorf("%w: action ID is empty", cognition.ErrInvalidAction)
	}
	receipt, found, err := loadActionRow(ctx, store.pool, store.schema, episode, actionID)
	if err != nil {
		return ActionReceipt{}, err
	}
	if !found {
		return ActionReceipt{}, ErrReceiptNotFound
	}
	return receipt.Receipt.clone(), nil
}
