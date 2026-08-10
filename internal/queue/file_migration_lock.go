package queue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const fileMigrationAdvisoryLockID int64 = 0x4f4d4e4944455801

const fileMigrationUnlockTimeout = 5 * time.Second

func acquireFileMigrationLock(ctx context.Context, conn *pgxpool.Conn) error {
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, fileMigrationAdvisoryLockID); err != nil {
		return fmt.Errorf("acquire file migration advisory lock: %w", err)
	}
	return nil
}

func releaseFileMigrationLock(conn *pgxpool.Conn) error {
	ctx, cancel := context.WithTimeout(context.Background(), fileMigrationUnlockTimeout)
	defer cancel()

	var unlocked bool
	err := conn.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, fileMigrationAdvisoryLockID).Scan(&unlocked)
	if err == nil && unlocked {
		conn.Release()
		return nil
	}

	closeErr := destroyLockedConnection(conn)
	if err != nil {
		return errors.Join(fmt.Errorf("release file migration advisory lock: %w", err), closeErr)
	}
	return errors.Join(fmt.Errorf("release file migration advisory lock: lock was not held"), closeErr)
}

func destroyLockedConnection(conn *pgxpool.Conn) error {
	ctx, cancel := context.WithTimeout(context.Background(), fileMigrationUnlockTimeout)
	defer cancel()
	raw := conn.Hijack()
	if err := raw.Close(ctx); err != nil {
		return fmt.Errorf("close file migration connection: %w", err)
	}
	return nil
}
