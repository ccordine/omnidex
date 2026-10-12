package workspacetransport

import "context"

// Lifetime attests one exact connection and cancels local native work when
// that connection closes. A later connection with the same binding never
// restores an earlier lifetime. The caller owns cancellation and cleanup.
func (hub *Hub) Lifetime(ctx context.Context, root, identity string) (context.Context, context.CancelFunc, error) {
	connection, err := hub.connection(ctx, root, identity)
	if err != nil {
		return nil, nil, err
	}
	if err := connection.attest(ctx); err != nil {
		return nil, nil, err
	}
	lifetime, cancel := context.WithCancelCause(ctx)
	release := func() { cancel(context.Canceled) }
	select {
	case <-connection.done:
		failure := connection.failure()
		cancel(failure)
		return nil, nil, failure
	case <-lifetime.Done():
		release()
		return nil, nil, lifetime.Err()
	default:
	}
	go func() {
		select {
		case <-connection.done:
			cancel(connection.failure())
		case <-lifetime.Done():
		}
	}()
	return lifetime, release, nil
}
