package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const workspaceMutationUnlockTimeout = 5 * time.Second

func acquireWorkspaceMutationLock(
	ctx context.Context,
	pool *pgxpool.Pool,
	command WorkspaceMutationCommand,
) (*pgxpool.Conn, error) {
	connection, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire workspace mutation lock connection: %w", err)
	}
	key := workspaceMutationLockKey(command)
	if _, err := connection.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, key); err != nil {
		connection.Release()
		return nil, fmt.Errorf("acquire workspace mutation advisory lock: %w", err)
	}
	return connection, nil
}

func releaseWorkspaceMutationLock(
	connection *pgxpool.Conn,
	command WorkspaceMutationCommand,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), workspaceMutationUnlockTimeout)
	defer cancel()
	var unlocked bool
	err := connection.QueryRow(
		ctx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`,
		workspaceMutationLockKey(command),
	).Scan(&unlocked)
	if err == nil && unlocked {
		connection.Release()
		return nil
	}
	closeErr := destroyLockedConnection(connection)
	if err != nil {
		return errors.Join(fmt.Errorf("release workspace mutation advisory lock: %w", err), closeErr)
	}
	return errors.Join(fmt.Errorf("release workspace mutation advisory lock: lock was not held"), closeErr)
}

func workspaceMutationLockKey(command WorkspaceMutationCommand) string {
	return "omnidex:workspace-mutation:" + command.Plan.WorkspaceID
}
