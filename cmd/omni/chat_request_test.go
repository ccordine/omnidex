package main

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestAwaitChatRequestCancelsInFlightRequestOnCtrlC(t *testing.T) {
	t.Parallel()

	signals := make(chan os.Signal, 1)
	started := make(chan struct{})
	canceled := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := awaitChatRequest(
			context.Background(),
			signals,
			func(ctx context.Context) (struct{}, error) {
				close(started)
				<-ctx.Done()
				close(canceled)
				return struct{}{}, ctx.Err()
			},
		)
		result <- err
	}()

	<-started
	signals <- os.Interrupt
	if err := <-result; !errors.Is(err, errChatRequestInterrupted) {
		t.Fatalf("Ctrl-C result = %v, want errChatRequestInterrupted", err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("Ctrl-C did not cancel the in-flight request context")
	}
}

func TestAwaitChatRequestCancelsInFlightRequestOnTermination(t *testing.T) {
	t.Parallel()

	signals := make(chan os.Signal, 1)
	started := make(chan struct{})
	canceled := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := awaitChatRequest(
			context.Background(),
			signals,
			func(ctx context.Context) (struct{}, error) {
				close(started)
				<-ctx.Done()
				close(canceled)
				return struct{}{}, ctx.Err()
			},
		)
		result <- err
	}()

	<-started
	signals <- syscall.SIGTERM
	if err := <-result; !errors.Is(err, errChatTerminated) {
		t.Fatalf("SIGTERM result = %v, want errChatTerminated", err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("SIGTERM did not cancel the in-flight request context")
	}
}
