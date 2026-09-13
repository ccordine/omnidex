package workspacetransport

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type pendingOperation struct {
	request   request
	result    chan responseData
	finished  chan struct{}
	abandoned chan struct{}
}

type remote struct {
	conn    *websocket.Conn
	binding binding
	serial  chan struct{}
	done    chan struct{}
	ready   chan struct{}
	mu      sync.Mutex
	nextID  uint64
	pending *pendingOperation
	err     error
	closed  bool
	once    sync.Once
}

func newRemote(conn *websocket.Conn, value binding) *remote {
	r := &remote{conn: conn, binding: value, serial: make(chan struct{}, 1), done: make(chan struct{}), ready: make(chan struct{})}
	r.serial <- struct{}{}
	go r.read()
	return r
}

func (r *remote) stop(err error) {
	r.once.Do(func() {
		r.mu.Lock()
		r.err = err
		r.closed = true
		r.mu.Unlock()
		_ = r.conn.Close()
		close(r.done)
	})
}

func (r *remote) failure() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return fmt.Errorf("client workspace connection closed: %w", r.err)
}

func (r *remote) read() {
	defer func() {
		r.mu.Lock()
		pending := r.pending
		r.pending = nil
		r.mu.Unlock()
		if pending != nil {
			close(pending.finished)
		}
	}()
	r.conn.SetPongHandler(func(string) error { return r.conn.SetReadDeadline(time.Now().Add(90 * time.Second)) })
	if err := r.conn.SetReadDeadline(time.Now().Add(90 * time.Second)); err != nil {
		r.stop(err)
		return
	}
	for {
		var reply response
		if err := readExact(r.conn, &reply); err != nil {
			r.stop(err)
			return
		}
		r.mu.Lock()
		pending := r.pending
		r.mu.Unlock()
		if pending == nil {
			r.stop(fmt.Errorf("workspace response has no pending request"))
			return
		}
		if err := reply.validate(pending.request); err != nil {
			r.stop(err)
			return
		}
		if reply.Identity != r.binding.Identity {
			r.stop(fmt.Errorf("client directory differs from its exact workspace binding: %s", reply.Error))
			return
		}
		data := responseData{response: reply}
		if pending.request.Kind == opReadFile && reply.Error == "" {
			var err error
			data.content, err = readContent(r.conn, reply.Entry.Size)
			if err != nil {
				r.stop(err)
				return
			}
		}
		if reply.Change == nil {
			r.mu.Lock()
			r.pending = nil
			r.mu.Unlock()
		}
		select {
		case pending.result <- data:
		case <-pending.abandoned:
			if reply.Change == nil {
				close(pending.finished)
			}
			return
		}
		if reply.Change == nil {
			close(pending.finished)
		}
	}
}

func (r *remote) attest(ctx context.Context) error {
	_, err := r.call(ctx, requestData{request: request{Kind: opAttest}}, nil)
	return err
}
