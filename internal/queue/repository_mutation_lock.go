package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const repositoryMutationUnlockTimeout = 5 * time.Second

func acquireRepositoryMutationLock(
	ctx context.Context,
	pool *pgxpool.Pool,
	command RepositoryMutationCommand,
) (*pgxpool.Conn, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire repository mutation lock connection: %w", err)
	}
	key := repositoryMutationLockKey(command)
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, key); err != nil {
		conn.Release()
		return nil, fmt.Errorf("acquire repository mutation advisory lock: %w", err)
	}
	return conn, nil
}

func releaseRepositoryMutationLock(
	conn *pgxpool.Conn,
	command RepositoryMutationCommand,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), repositoryMutationUnlockTimeout)
	defer cancel()
	var unlocked bool
	err := conn.QueryRow(
		ctx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`,
		repositoryMutationLockKey(command),
	).Scan(&unlocked)
	if err == nil && unlocked {
		conn.Release()
		return nil
	}
	closeErr := destroyLockedConnection(conn)
	if err != nil {
		return errors.Join(fmt.Errorf("release repository mutation advisory lock: %w", err), closeErr)
	}
	return errors.Join(fmt.Errorf("release repository mutation advisory lock: lock was not held"), closeErr)
}

func repositoryMutationLockKey(command RepositoryMutationCommand) string {
	return fmt.Sprintf("omnidex:repository-mutation:%d:%s", command.JobID, command.StageID)
}
