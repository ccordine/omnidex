package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ConnectLegacyPublicCutover opens the one-connection administrative boundary
// used only by the explicit legacy-public preservation command. It deliberately
// does not bootstrap a runtime schema or accept URL-selected search paths.
func ConnectLegacyPublicCutover(
	ctx context.Context,
	databaseURL string,
) (*pgxpool.Pool, error) {
	if ctx == nil {
		return nil, fmt.Errorf("legacy public cutover connection requires context")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse legacy public cutover database URL: %w", err)
	}
	_, directSearchPath := cfg.ConnConfig.RuntimeParams["search_path"]
	if directSearchPath || strings.Contains(
		strings.ToLower(cfg.ConnConfig.RuntimeParams["options"]), "search_path",
	) {
		return nil, fmt.Errorf("DATABASE_URL search_path is forbidden during legacy public cutover")
	}
	cfg.MaxConns = 1
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open legacy public cutover connection: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping legacy public cutover database: %w", err)
	}
	return pool, nil
}
