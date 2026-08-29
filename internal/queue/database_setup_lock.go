package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const databaseSetupAdvisoryLockID int64 = 0x4f4d4e4944455801

const databaseSetupUnlockTimeout = 5 * time.Second

func acquireDatabaseSetupLock(ctx context.Context, conn *pgxpool.Conn) error {
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, databaseSetupAdvisoryLockID); err != nil {
		return fmt.Errorf("acquire database setup advisory lock: %w", err)
	}
	return nil
}

func releaseDatabaseSetupLock(conn *pgxpool.Conn) error {
	ctx, cancel := context.WithTimeout(context.Background(), databaseSetupUnlockTimeout)
	defer cancel()

	var unlocked bool
	err := conn.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, databaseSetupAdvisoryLockID).Scan(&unlocked)
	if err == nil && unlocked {
		conn.Release()
		return nil
	}

	closeErr := destroyLockedConnection(conn)
	if err != nil {
		return errors.Join(fmt.Errorf("release database setup advisory lock: %w", err), closeErr)
	}
	return errors.Join(fmt.Errorf("release database setup advisory lock: lock was not held"), closeErr)
}

func destroyLockedConnection(conn *pgxpool.Conn) error {
	ctx, cancel := context.WithTimeout(context.Background(), databaseSetupUnlockTimeout)
	defer cancel()
	raw := conn.Hijack()
	if err := raw.Close(ctx); err != nil {
		return fmt.Errorf("close locked database connection: %w", err)
	}
	return nil
}
