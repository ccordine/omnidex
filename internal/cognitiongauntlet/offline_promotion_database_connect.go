package cognitiongauntlet

import (
	"context"
	"fmt"
	"net/url"

	"github.com/jackc/pgx/v5/pgxpool"
)

func promotionPool(
	ctx context.Context,
	databaseURL string,
	schema string,
) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	return pgxpool.NewWithConfig(ctx, config)
}

func restrictedDatabaseURL(baseURL string, roleName string, password string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") ||
		parsed.Host == "" || roleName == "" || password == "" {
		return "", fmt.Errorf("offline promotion database URL or restricted credential is invalid")
	}
	parsed.User = url.UserPassword(roleName, password)
	return parsed.String(), nil
}
