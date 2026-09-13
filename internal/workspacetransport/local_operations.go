package workspacetransport

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/workspace"
)

func (l *Local) serve() {
	defer l.cleanup()
	requests := make(chan requestData, 1)
	go l.readRequests(requests)
	for {
		select {
		case <-l.ctx.Done():
			return
		case data := <-requests:
			if err := l.ctx.Err(); err != nil {
				return
			}
			if err := l.execute(data); err != nil {
				l.stop(err)
				return
			}
		}
	}
}

// Reading continues during filesystem operations so EOF immediately cancels
// the operation context. Only one request may be active on the connection.
func (l *Local) readRequests(requests chan<- requestData) {
	for {
		var req request
		if err := readExact(l.conn, &req); err != nil {
			l.stop(err)
			return
		}
		if err := req.validate(); err != nil {
			l.stop(err)
			return
		}
		if l.nextID == ^uint64(0) || req.ID != l.nextID+1 || !l.busy.CompareAndSwap(false, true) {
			l.stop(fmt.Errorf("workspace request is concurrent or out of sequence"))
			return
		}
		l.nextID = req.ID
		data, err := readRequestData(l.conn, req)
		if err != nil {
			l.stop(err)
			return
		}
		select {
		case requests <- data:
		case <-l.ctx.Done():
			return
		}
	}
}

func (l *Local) execute(data requestData) error {
	if err := l.attest(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(l.ctx, transferTimeout)
	defer cancel()
	reply := response{ID: data.ID, Identity: l.identity}
	var content []byte
	var operationErr error
	if data.Lease != 0 && (l.fence == nil || data.Lease != l.lease) {
		operationErr = fmt.Errorf("workspace request has no exact acquired lease")
	} else {
		content, operationErr = l.operate(ctx, cancel, data, &reply)
	}
	if err := l.attest(); err != nil {
		return err
	}
	setResponseError(&reply, operationErr)
	l.busy.Store(false)
	if err := writeExact(l.conn, reply); err != nil {
		return err
	}
	return writeContent(l.conn, content)
}

func (l *Local) operate(ctx context.Context, cancel context.CancelFunc, data requestData, reply *response) ([]byte, error) {
	var reader workspace.Reader = l.handle
	if data.Lease != 0 {
		reader = l.fence
	}
	switch data.Kind {
	case opAttest:
		if data.Lease != 0 {
			return nil, l.fence.Reattest(l.handle.Path())
		}
		return nil, nil
	case opStat:
		entry, err := reader.Stat(ctx, data.Path)
		if err == nil {
			reply.Entry = &entry
		}
		return nil, err
	case opReadFile:
		file, err := reader.ReadFile(ctx, data.Path)
		if err != nil {
			return nil, err
		}
		reply.Entry = &file.Entry
		return file.Content, nil
	case opReadDirectory:
		page, err := reader.ReadDirectory(ctx, data.Path, data.After, data.Limit)
		if err == nil {
			reply.Page = &page
		}
		return nil, err
	case opAcquire:
		if l.fence != nil {
			return nil, workspace.ErrWorkspaceBusy
		}
		fence, err := workspace.TryAcquireMutationFence(ctx, l.handle.Path())
		if err != nil {
			return nil, err
		}
		l.fence, l.lease = fence, data.ID
		return nil, nil
	case opPrepare:
		if l.prepared != nil {
			return nil, fmt.Errorf("workspace already has an unapplied preparation")
		}
		prepared, err := l.fence.Prepare(ctx, data.desired, data.expected)
		if err != nil {
			return nil, err
		}
		l.prepared, l.preparedID = prepared, data.ID
		return nil, nil
	case opApply:
		if l.prepared == nil || l.preparedID != data.Prepared {
			return nil, fmt.Errorf("workspace apply has no exact retained preparation")
		}
		prepared := l.prepared
		l.prepared, l.preparedID = nil, 0
		var writeErr error
		result, err := prepared.ApplyVerified(ctx, func(change workspace.Change) {
			if writeErr != nil {
				return
			}
			writeErr = writeExact(l.conn, response{ID: data.ID, Identity: l.identity, Change: &change})
			if writeErr != nil {
				cancel()
			}
		})
		reply.AppliedChanges = len(result.Changes)
		return nil, errors.Join(err, writeErr)
	case opRelease:
		err := l.fence.Release()
		l.fence, l.lease, l.prepared, l.preparedID = nil, 0, nil, 0
		return nil, err
	default:
		return nil, fmt.Errorf("workspace operation is unregistered")
	}
}
