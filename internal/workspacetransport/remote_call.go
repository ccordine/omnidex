package workspacetransport

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/workspace"
)

func (r *remote) call(ctx context.Context, data requestData, observe func(workspace.Change) error) (responseData, error) {
	if ctx == nil {
		return responseData{}, fmt.Errorf("workspace operation requires context")
	}
	timeout := transferTimeout
	if data.Kind == opAttest {
		timeout = operationTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case <-r.done:
		return responseData{}, r.failure()
	case <-ctx.Done():
		return responseData{}, ctx.Err()
	case <-r.serial:
	}
	defer func() { r.serial <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return responseData{}, err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return responseData{}, r.failure()
	}
	if r.nextID == ^uint64(0) {
		r.mu.Unlock()
		return responseData{}, fmt.Errorf("workspace request sequence is exhausted")
	}
	data.ID = r.nextID + 1
	if err := data.request.validate(); err != nil {
		r.mu.Unlock()
		return responseData{}, err
	}
	r.nextID = data.ID
	pending := &pendingOperation{request: data.request, result: make(chan responseData, 1), finished: make(chan struct{}), abandoned: make(chan struct{})}
	r.pending = pending
	r.mu.Unlock()
	defer close(pending.abandoned)
	stop := context.AfterFunc(ctx, func() { r.stop(ctx.Err()) })
	defer stop()
	if err := writeRequestData(r.conn, data); err != nil {
		r.stop(err)
		return responseData{}, drainPendingChanges(pending, observe, err)
	}
	for {
		select {
		case <-ctx.Done():
			r.stop(ctx.Err())
			return responseData{}, drainPendingChanges(pending, observe, ctx.Err())
		case <-r.done:
			return responseData{}, drainPendingChanges(pending, observe, r.failure())
		case reply := <-pending.result:
			if reply.Change != nil {
				if observe == nil {
					err := fmt.Errorf("workspace change has no owning observer")
					r.stop(err)
					return responseData{}, err
				}
				if err := observe(*reply.Change); err != nil {
					r.stop(err)
					return responseData{}, err
				}
				continue
			}
			if reply.Error != "" {
				return reply, &remoteError{kind: reply.ErrorKind, message: reply.Error}
			}
			return reply, nil
		}
	}
}

// The socket reader can already hold a validated change when cancellation
// closes the connection. Deliver that retained evidence before returning.
func drainPendingChanges(pending *pendingOperation, observe func(workspace.Change) error, failure error) error {
	consume := func(reply responseData) error {
		if reply.Change == nil {
			return nil
		}
		if observe == nil {
			return fmt.Errorf("workspace change has no owning observer")
		}
		return observe(*reply.Change)
	}
	for {
		select {
		case reply := <-pending.result:
			if err := consume(reply); err != nil {
				return err
			}
		case <-pending.finished:
			select {
			case reply := <-pending.result:
				if err := consume(reply); err != nil {
					return err
				}
			default:
			}
			return failure
		}
	}
}
