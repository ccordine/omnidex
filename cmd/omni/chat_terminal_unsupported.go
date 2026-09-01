//go:build !darwin && !linux && !windows

package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
)

type terminalContextReader struct {
	ctx  context.Context
	file *os.File
	gate sync.Mutex
}

func newTerminalContextReader(ctx context.Context, file *os.File) *terminalContextReader {
	return &terminalContextReader{ctx: ctx, file: file}
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
	return 0, fmt.Errorf("interactive omni chat terminals are unsupported on %s", runtime.GOOS)
}

func enableTerminalInterruptSignal(_ int) error {
	return fmt.Errorf("interactive omni chat terminals are unsupported on %s", runtime.GOOS)
}
