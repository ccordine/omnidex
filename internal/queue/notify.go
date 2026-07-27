package queue

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrRepositoryNotConfigured = errors.New("postgres repository is not configured")

type NotifyConn interface {
	Exec(ctx context.Context, sql string, args ...any) error
	WaitForNotification(ctx context.Context) (*pgconn.Notification, error)
	Close(ctx context.Context) error
}

type notifyConn struct {
	conn *pgxpool.Conn
}

func (n *notifyConn) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := n.conn.Exec(ctx, sql, args...)
	return err
}

func (n *notifyConn) WaitForNotification(ctx context.Context) (*pgconn.Notification, error) {
	return n.conn.Conn().WaitForNotification(ctx)
}

func (n *notifyConn) Close(ctx context.Context) error {
	n.conn.Release()
	return nil
}

func (r *Repository) AcquireNotifyConn(ctx context.Context) (NotifyConn, error) {
	if r == nil || r.pool == nil {
		return nil, ErrRepositoryNotConfigured
	}
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	return &notifyConn{conn: conn}, nil
}
