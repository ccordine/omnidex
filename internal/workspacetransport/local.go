package workspacetransport

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/workspace"
)

type Local struct {
	conn       *websocket.Conn
	handle     *projectroot.DirectoryHandle
	clientID   string
	identity   string
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	once       sync.Once
	mu         sync.Mutex
	err        error
	closeErr   error
	nextID     uint64
	busy       atomic.Bool
	fence      *workspace.MutationFence
	lease      uint64
	prepared   workspace.Prepared
	preparedID uint64
}

// Connect owns the socket and borrows the caller's retained directory. It
// never reopens a root selected before configuration or network setup.
func Connect(ctx context.Context, conn *websocket.Conn, handle *projectroot.DirectoryHandle, clientID string) (*Local, error) {
	if conn == nil {
		return nil, fmt.Errorf("connect client workspace requires context and transport")
	}
	if ctx == nil || handle == nil {
		return nil, errors.Join(fmt.Errorf("connect client workspace requires context and retained directory"), conn.Close())
	}
	lifetime, cancel := context.WithCancel(ctx)
	l := &Local{conn: conn, handle: handle, clientID: clientID, ctx: lifetime, cancel: cancel, done: make(chan struct{})}
	stop := context.AfterFunc(lifetime, func() { l.stop(nil) })
	if err := l.initialize(handle.Path()); err != nil {
		l.stop(err)
		l.cleanup()
		stop()
		return nil, errors.Join(err, l.closeErr)
	}
	go func() { defer stop(); l.serve() }()
	return l, nil
}

func (l *Local) initialize(root string) error {
	directoryID, err := l.handle.Identity()
	if err != nil {
		return err
	}
	l.identity, err = projectroot.ClientWorkspaceIdentity(l.clientID, directoryID)
	if err != nil {
		return err
	}
	l.conn.SetReadLimit(MaxFrameBytes)
	if err := l.conn.SetReadDeadline(time.Now().Add(operationTimeout)); err != nil {
		return err
	}
	if err := writeExact(l.conn, binding{Root: root, Identity: l.identity}); err != nil {
		return err
	}
	var req request
	if err := readExact(l.conn, &req); err != nil {
		return err
	}
	if err := req.validate(); err != nil {
		return err
	}
	if req.ID != 1 || req.Kind != opAttest || req.Lease != 0 {
		return fmt.Errorf("workspace connection requires initial directory attestation")
	}
	l.nextID = req.ID
	if err := l.attest(); err != nil {
		return err
	}
	if err := writeExact(l.conn, response{ID: req.ID, Identity: l.identity}); err != nil {
		return err
	}
	var acknowledgement ready
	if err := readExact(l.conn, &acknowledgement); err != nil {
		return err
	}
	if acknowledgement.Identity != l.identity {
		return fmt.Errorf("workspace connection acknowledgement differs from client binding")
	}
	l.conn.SetPingHandler(func(value string) error {
		if err := l.conn.SetReadDeadline(time.Now().Add(90 * time.Second)); err != nil {
			return err
		}
		return l.conn.WriteControl(websocket.PongMessage, []byte(value), time.Now().Add(operationTimeout))
	})
	return l.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
}

func (l *Local) attest() error {
	directoryID, err := l.handle.Identity()
	if err != nil {
		return err
	}
	identity, err := projectroot.ClientWorkspaceIdentity(l.clientID, directoryID)
	if err != nil {
		return err
	}
	if identity != l.identity {
		return fmt.Errorf("client directory changed from its connection binding")
	}
	return nil
}

func (l *Local) stop(err error) {
	l.once.Do(func() {
		l.mu.Lock()
		l.err = err
		l.mu.Unlock()
		l.cancel()
		l.closeErr = l.conn.Close()
	})
}

func (l *Local) cleanup() {
	l.stop(nil)
	if l.fence != nil {
		l.closeErr = errors.Join(l.closeErr, l.fence.Release())
	}
	close(l.done)
}

func (l *Local) Identity() string      { return l.identity }
func (l *Local) Done() <-chan struct{} { return l.done }
func (l *Local) Err() error            { l.mu.Lock(); defer l.mu.Unlock(); return l.err }
func (l *Local) Close() error          { l.stop(nil); <-l.done; return l.closeErr }
