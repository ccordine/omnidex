package main

import (
	"context"
	"errors"
	"os"
	"syscall"
)

var (
	errChatRequestInterrupted = errors.New("interactive request interrupted")
	errChatTerminated         = errors.New("interactive session terminated")
)

type chatRequestResult[T any] struct {
	value T
	err   error
}

func awaitChatRequest[T any](
	ctx context.Context,
	signals <-chan os.Signal,
	request func(context.Context) (T, error),
) (T, error) {
	var zero T
	requestContext, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan chatRequestResult[T], 1)
	go func() {
		value, err := request(requestContext)
		result <- chatRequestResult[T]{value: value, err: err}
	}()
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case signal := <-signals:
		cancel()
		if signal == syscall.SIGTERM {
			return zero, errChatTerminated
		}
		return zero, errChatRequestInterrupted
	case completed := <-result:
		return completed.value, completed.err
	}
}

// awaitAuthoritativeChatRequest resolves an already ambiguous idempotent
// operation. Further Ctrl-C signals preserve the original interrupt intent;
// SIGTERM still terminates the client.
func awaitAuthoritativeChatRequest[T any](
	ctx context.Context,
	signals <-chan os.Signal,
	request func(context.Context) (T, error),
) (T, error) {
	var zero T
	requestContext, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan chatRequestResult[T], 1)
	go func() {
		value, err := request(requestContext)
		result <- chatRequestResult[T]{value: value, err: err}
	}()
	for {
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case signal := <-signals:
			if signal == syscall.SIGTERM {
				cancel()
				return zero, errChatTerminated
			}
		case completed := <-result:
			return completed.value, completed.err
		}
	}
}
