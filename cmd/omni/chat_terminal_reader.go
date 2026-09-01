//go:build darwin || linux

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

const terminalInputPollMilliseconds = 100

type terminalContextReader struct {
	ctx  context.Context
	file *os.File
	fd   int32
	gate sync.Mutex
}

func newTerminalContextReader(ctx context.Context, file *os.File) *terminalContextReader {
	return &terminalContextReader{ctx: ctx, file: file, fd: int32(file.Fd())}
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
	descriptors := []unix.PollFd{{
		Fd: reader.fd, Events: unix.POLLIN | unix.POLLHUP | unix.POLLERR,
	}}
	ready, err := unix.Poll(descriptors, terminalInputPollMilliseconds)
	if errors.Is(err, unix.EINTR) {
		return 0, errPlanReviewInputSourceIdle
	}
	if err != nil {
		return 0, fmt.Errorf("poll terminal input: %w", err)
	}
	if ready == 0 {
		return 0, errPlanReviewInputSourceIdle
	}
	if descriptors[0].Revents&unix.POLLNVAL != 0 {
		return 0, fmt.Errorf("terminal input descriptor became invalid")
	}
	return reader.file.Read(buffer)
}
