package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func createOfflineDatabaseAuthorities(
	ctx context.Context,
	admin *pgxpool.Pool,
	runtimeSchema string,
	hostSchema string,
	inferenceRole string,
	inferencePassword string,
	hostRole string,
	hostPassword string,
) error {
	var inferencePasswordLiteral string
	if err := admin.QueryRow(ctx, `SELECT quote_literal($1)`, inferencePassword).Scan(&inferencePasswordLiteral); err != nil {
		return err
	}
	var hostPasswordLiteral string
	if err := admin.QueryRow(ctx, `SELECT quote_literal($1)`, hostPassword).Scan(&hostPasswordLiteral); err != nil {
		return err
	}
	runtimeID := pgx.Identifier{runtimeSchema}.Sanitize()
	hostID := pgx.Identifier{hostSchema}.Sanitize()
	inferenceRoleID := pgx.Identifier{inferenceRole}.Sanitize()
	hostRoleID := pgx.Identifier{hostRole}.Sanitize()
	commands := []string{
		"CREATE SCHEMA " + runtimeID,
		"CREATE SCHEMA " + hostID,
		"REVOKE ALL ON SCHEMA " + runtimeID + " FROM PUBLIC",
		"REVOKE ALL ON SCHEMA " + hostID + " FROM PUBLIC",
		"CREATE ROLE " + inferenceRoleID + " NOLOGIN PASSWORD " + inferencePasswordLiteral +
			" NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION CONNECTION LIMIT 8",
		"CREATE ROLE " + hostRoleID + " NOLOGIN PASSWORD " + hostPasswordLiteral +
			" NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION CONNECTION LIMIT 8",
	}
	for _, command := range commands {
		if _, err := admin.Exec(ctx, command); err != nil {
			return fmt.Errorf("create offline database authority: %w", err)
		}
	}
	return nil
}

func grantOfflineHostAuthority(
	ctx context.Context,
	admin *pgxpool.Pool,
	schemaName string,
	roleName string,
) error {
	schema := pgx.Identifier{schemaName}.Sanitize()
	role := pgx.Identifier{roleName}.Sanitize()
	commands := []string{
		"GRANT USAGE ON SCHEMA " + schema + " TO " + role,
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA " + schema + " TO " + role,
	}
	for _, command := range commands {
		if _, err := admin.Exec(ctx, command); err != nil {
			return fmt.Errorf("grant offline host authority: %w", err)
		}
	}
	return nil
}

func grantOfflineRuntimeAuthority(
	ctx context.Context,
	admin *pgxpool.Pool,
	schemaName string,
	roleName string,
) error {
	schema := pgx.Identifier{schemaName}.Sanitize()
	role := pgx.Identifier{roleName}.Sanitize()
	commands := []string{
		"GRANT USAGE ON SCHEMA " + schema + " TO " + role,
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA " + schema + " TO " + role,
		"GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA " + schema + " TO " + role,
	}
	for _, command := range commands {
		if _, err := admin.Exec(ctx, command); err != nil {
			return fmt.Errorf("grant offline runtime authority: %w", err)
		}
	}
	return nil
}

func revokeOfflineInferenceLogin(
	ctx context.Context,
	admin *pgxpool.Pool,
	roleName string,
) error {
	return revokeOfflineProcessLogin(ctx, admin, roleName, "inference")
}

func enableOfflineInferenceLogin(
	ctx context.Context,
	admin *pgxpool.Pool,
	roleName string,
) error {
	return enableOfflineProcessLogin(ctx, admin, roleName, "inference")
}

func revokeOfflineProcessLogin(
	ctx context.Context,
	admin *pgxpool.Pool,
	roleName string,
	label string,
) error {
	if _, err := admin.Exec(ctx, "ALTER ROLE "+pgx.Identifier{roleName}.Sanitize()+" NOLOGIN"); err != nil {
		return fmt.Errorf("revoke offline %s login: %w", label, err)
	}
	return nil
}

func enableOfflineProcessLogin(
	ctx context.Context,
	admin *pgxpool.Pool,
	roleName string,
	label string,
) error {
	if _, err := admin.Exec(ctx, "ALTER ROLE "+pgx.Identifier{roleName}.Sanitize()+" LOGIN"); err != nil {
		return fmt.Errorf("enable offline %s login: %w", label, err)
	}
	return nil
}
