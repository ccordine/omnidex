package workspacetransport

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Hub owns live connections only. PostgreSQL owns their channel/job bindings;
// filesystem observations are always obtained from the exact connected client.
type Hub struct {
	mu          sync.Mutex
	connections map[string]*remote
}

func NewHub() *Hub { return &Hub{connections: make(map[string]*remote)} }

func (hub *Hub) Serve(ctx context.Context, conn *websocket.Conn) error {
	if hub == nil || ctx == nil || conn == nil {
		return fmt.Errorf("workspace connection requires hub, context, and transport")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	conn.SetReadLimit(MaxFrameBytes)
	if err := conn.SetReadDeadline(time.Now().Add(operationTimeout)); err != nil {
		return err
	}
	var value binding
	if err := readExact(conn, &value); err != nil {
		return err
	}
	if err := value.validate(); err != nil {
		return err
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return err
	}
	r := newRemote(conn, value)
	defer r.stop(fmt.Errorf("workspace connection service ended"))
	if err := r.attest(ctx); err != nil {
		return err
	}
	hub.mu.Lock()
	if existing, exists := hub.connections[value.Identity]; exists {
		select {
		case <-existing.done:
			delete(hub.connections, value.Identity)
		default:
			hub.mu.Unlock()
			err := fmt.Errorf("client workspace already has an active connection")
			_ = conn.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.ClosePolicyViolation, err.Error()),
				time.Now().Add(operationTimeout))
			return err
		}
	}
	hub.connections[value.Identity] = r
	hub.mu.Unlock()
	defer func() {
		hub.mu.Lock()
		if hub.connections[value.Identity] == r {
			delete(hub.connections, value.Identity)
		}
		hub.mu.Unlock()
	}()
	if err := writeExact(conn, ready{Identity: value.Identity}); err != nil {
		return err
	}
	close(r.ready)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.done:
			return r.failure()
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(operationTimeout)); err != nil {
				r.stop(err)
			}
		}
	}
}

func (hub *Hub) Require(ctx context.Context, root, identity string) error {
	r, err := hub.connection(ctx, root, identity)
	if err != nil {
		return err
	}
	return r.attest(ctx)
}

func (hub *Hub) connection(ctx context.Context, root, identity string) (*remote, error) {
	if hub == nil || ctx == nil {
		return nil, fmt.Errorf("client workspace transport requires hub and context")
	}
	value := binding{Root: root, Identity: identity}
	if err := value.validate(); err != nil {
		return nil, err
	}
	hub.mu.Lock()
	r := hub.connections[identity]
	hub.mu.Unlock()
	if r == nil {
		return nil, fmt.Errorf("client workspace connection is unavailable")
	}
	if r.binding != value {
		return nil, fmt.Errorf("workspace path differs from its exact client connection")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.done:
		return nil, r.failure()
	case <-r.ready:
	}
	return r, nil
}
