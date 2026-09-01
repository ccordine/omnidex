//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"sync"

	"golang.org/x/sys/windows"
)

const terminalInputPollMilliseconds = 100

type terminalContextReader struct {
	ctx    context.Context
	file   *os.File
	handle windows.Handle
	gate   sync.Mutex
}

func newTerminalContextReader(ctx context.Context, file *os.File) *terminalContextReader {
	return &terminalContextReader{ctx: ctx, file: file, handle: windows.Handle(file.Fd())}
}

func (reader *terminalContextReader) Read(buffer []byte) (int, error) {
	reader.LockInput()
	defer reader.UnlockInput()
	return reader.ReadInputLocked(buffer)
}

func (reader *terminalContextReader) LockInput() {
	reader.gate.Lock()
}

func (reader *terminalContextReader) UnlockInput() {
	reader.gate.Unlock()
}

func (reader *terminalContextReader) ReadInputLocked(buffer []byte) (int, error) {
	if reader == nil || reader.ctx == nil || reader.file == nil {
		return 0, fmt.Errorf("terminal input reader is unavailable")
	}
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	result, err := windows.WaitForSingleObject(reader.handle, terminalInputPollMilliseconds)
	if err != nil {
		return 0, fmt.Errorf("wait for terminal input: %w", err)
	}
	switch result {
	case windows.WAIT_OBJECT_0:
		return reader.file.Read(buffer)
	case uint32(windows.WAIT_TIMEOUT):
		return 0, errPlanReviewInputSourceIdle
	default:
		return 0, fmt.Errorf("wait for terminal input returned state %d", result)
	}
}
