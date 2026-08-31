package datasource

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/omni"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ConnectReadOnly(ctx context.Context, conn Connection) (*pgxpool.Pool, error) {
	if !conn.ReadOnly {
		return nil, fmt.Errorf("only read-only data sources are supported")
	}
	driver := strings.ToLower(strings.TrimSpace(conn.Driver))
	if driver != "" && driver != "postgres" {
		return nil, fmt.Errorf("only postgres data sources are supported")
	}
	dsn, err := BuildPostgresDSN(conn)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if _, err := omni.NewPgxMemoryRunner(pool).Query(ctx, "SELECT 1"); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func InspectSchema(ctx context.Context, conn Connection) ([]omni.DBSchemaTable, error) {
	pool, err := ConnectReadOnly(ctx, conn)
	if err != nil {
		return nil, err
	}
	defer pool.Close()
	return omni.InspectPostgresSchema(ctx, omni.NewPgxMemoryRunner(pool))
}
